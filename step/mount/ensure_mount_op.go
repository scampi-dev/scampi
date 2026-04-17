// SPDX-License-Identifier: GPL-3.0-only

package mount

import (
	"context"
	"fmt"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const ensureMountID = "step.mount"

type ensureMountOp struct {
	sharedops.BaseOp
	src   string
	dest  string
	fstyp FsType
	opts  string
	state State
}

// Check
// -----------------------------------------------------------------------------

func (op *ensureMountOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](ensureMountID, tgt)
	fsTgt := target.Must[target.Filesystem](ensureMountID, tgt)

	if op.state != StateAbsent {
		if err := op.checkTools(ctx, cmdr); err != nil {
			return spec.CheckUnsatisfied, nil, err
		}
	}

	inFstab, fstabEntry := op.findFstabEntry(ctx, fsTgt)
	isMounted := op.isMounted(ctx, cmdr)

	switch op.state {
	case StateMounted:
		if inFstab && fstabEntry == op.fstabLine() && isMounted {
			return spec.CheckSatisfied, nil, nil
		}
		var drift []spec.DriftDetail
		if !inFstab {
			drift = append(drift, spec.DriftDetail{
				Field: "fstab", Desired: "present",
			})
		} else if fstabEntry != op.fstabLine() {
			drift = append(drift, spec.DriftDetail{
				Field: "fstab", Current: fstabEntry, Desired: op.fstabLine(),
			})
		}
		if !isMounted {
			drift = append(drift, spec.DriftDetail{
				Field: "mounted", Current: "no", Desired: "yes",
			})
		}
		return spec.CheckUnsatisfied, drift, nil

	case StateUnmounted:
		if inFstab && fstabEntry == op.fstabLine() && !isMounted {
			return spec.CheckSatisfied, nil, nil
		}
		var drift []spec.DriftDetail
		if !inFstab {
			drift = append(drift, spec.DriftDetail{
				Field: "fstab", Desired: "present",
			})
		} else if fstabEntry != op.fstabLine() {
			drift = append(drift, spec.DriftDetail{
				Field: "fstab", Current: fstabEntry, Desired: op.fstabLine(),
			})
		}
		if isMounted {
			drift = append(drift, spec.DriftDetail{
				Field: "mounted", Current: "yes", Desired: "no",
			})
		}
		return spec.CheckUnsatisfied, drift, nil

	case StateAbsent:
		if !inFstab && !isMounted {
			return spec.CheckSatisfied, nil, nil
		}
		var drift []spec.DriftDetail
		if inFstab {
			drift = append(drift, spec.DriftDetail{
				Field: "fstab", Current: "present", Desired: "absent",
			})
		}
		if isMounted {
			drift = append(drift, spec.DriftDetail{
				Field: "mounted", Current: "yes", Desired: "no",
			})
		}
		return spec.CheckUnsatisfied, drift, nil
	}

	return spec.CheckSatisfied, nil, nil
}

// Execute
// -----------------------------------------------------------------------------

func (op *ensureMountOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](ensureMountID, tgt)
	fsTgt := target.Must[target.Filesystem](ensureMountID, tgt)

	switch op.state {
	case StateMounted:
		if err := op.ensureFstab(ctx, fsTgt); err != nil {
			return spec.Result{}, err
		}
		if err := op.ensureMountPoint(ctx, fsTgt); err != nil {
			return spec.Result{}, err
		}
		if !op.isMounted(ctx, cmdr) {
			if err := op.doMount(ctx, cmdr); err != nil {
				return spec.Result{}, err
			}
		}

	case StateUnmounted:
		if err := op.ensureFstab(ctx, fsTgt); err != nil {
			return spec.Result{}, err
		}
		if op.isMounted(ctx, cmdr) {
			if err := op.doUnmount(ctx, cmdr); err != nil {
				return spec.Result{}, err
			}
		}

	case StateAbsent:
		if op.isMounted(ctx, cmdr) {
			if err := op.doUnmount(ctx, cmdr); err != nil {
				return spec.Result{}, err
			}
		}
		if err := op.removeFstab(ctx, fsTgt); err != nil {
			return spec.Result{}, err
		}
	}

	return spec.Result{Changed: true}, nil
}

func (ensureMountOp) RequiredCapabilities() capability.Capability {
	return capability.Filesystem | capability.Command
}

// fstab helpers
// -----------------------------------------------------------------------------

const fstabPath = "/etc/fstab"

func (op *ensureMountOp) fstabLine() string {
	return fmt.Sprintf("%s %s %s %s 0 0", op.src, op.dest, op.fstyp.String(), op.opts)
}

func (op *ensureMountOp) findFstabEntry(
	ctx context.Context,
	fsTgt target.Filesystem,
) (bool, string) {
	data, err := fsTgt.ReadFile(ctx, fstabPath)
	if err != nil {
		return false, ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == op.dest {
			return true, line
		}
	}
	return false, ""
}

func (op *ensureMountOp) ensureFstab(
	ctx context.Context,
	fsTgt target.Filesystem,
) error {
	data, err := fsTgt.ReadFile(ctx, fstabPath)
	if err != nil {
		return sharedops.DiagnoseTargetError(err)
	}

	content := string(data)
	desired := op.fstabLine()

	var lines []string
	found := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 && fields[1] == op.dest {
				lines = append(lines, desired)
				found = true
				continue
			}
		}
		lines = append(lines, line)
	}

	if !found {
		if !strings.HasSuffix(content, "\n") {
			lines = append(lines, "")
		}
		lines = append(lines, desired)
	}

	return fsTgt.WriteFile(ctx, fstabPath, []byte(strings.Join(lines, "\n")))
}

func (op *ensureMountOp) removeFstab(
	ctx context.Context,
	fsTgt target.Filesystem,
) error {
	data, err := fsTgt.ReadFile(ctx, fstabPath)
	if err != nil {
		return sharedops.DiagnoseTargetError(err)
	}

	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 && fields[1] == op.dest {
				continue
			}
		}
		lines = append(lines, line)
	}

	return fsTgt.WriteFile(ctx, fstabPath, []byte(strings.Join(lines, "\n")))
}

// mount/umount helpers
// -----------------------------------------------------------------------------

func (op *ensureMountOp) isMounted(ctx context.Context, cmdr target.Command) bool {
	result, err := cmdr.RunCommand(ctx, fmt.Sprintf("findmnt --target %s --noheadings", op.dest))
	return err == nil && result.ExitCode == 0
}

func (op *ensureMountOp) doMount(ctx context.Context, cmdr target.Command) error {
	result, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("mount %s", op.dest))
	if err != nil {
		return sharedops.DiagnoseTargetError(err)
	}
	if result.ExitCode != 0 {
		return MountCommandError{
			Op:     "mount",
			Dest:   op.dest,
			Stderr: result.Stderr,
		}
	}
	return nil
}

func (op *ensureMountOp) doUnmount(ctx context.Context, cmdr target.Command) error {
	result, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("umount %s", op.dest))
	if err != nil {
		return sharedops.DiagnoseTargetError(err)
	}
	if result.ExitCode != 0 {
		return MountCommandError{
			Op:     "umount",
			Dest:   op.dest,
			Stderr: result.Stderr,
		}
	}
	return nil
}

func (op *ensureMountOp) ensureMountPoint(
	ctx context.Context,
	fsTgt target.Filesystem,
) error {
	// Mkdir is a no-op if the directory already exists.
	_ = fsTgt.Mkdir(ctx, op.dest, 0o755)
	return nil
}

// Tool detection
// -----------------------------------------------------------------------------

func (op *ensureMountOp) checkTools(ctx context.Context, cmdr target.Command) error {
	if !op.fstyp.NeedsHelper() {
		return nil
	}
	result, err := cmdr.RunCommand(ctx, "which "+op.fstyp.HelperBinary())
	if err != nil || result.ExitCode != 0 {
		return MissingToolError{FsType: op.fstyp.String()}
	}
	return nil
}

// OpDescription
// -----------------------------------------------------------------------------

type ensureMountDesc struct {
	State State
	Src   string
	Dest  string
	Type  string
}

func (d ensureMountDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureMountID,
		Text: `mount {{.State}} {{.Dest}} ({{.Type}} from {{.Src}})`,
		Data: d,
	}
}

func (op *ensureMountOp) OpDescription() spec.OpDescription {
	return ensureMountDesc{
		State: op.state,
		Src:   op.src,
		Dest:  op.dest,
		Type:  op.fstyp.String(),
	}
}

func (op *ensureMountOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "src", Value: op.src},
		{Label: "dest", Value: op.dest},
		{Label: "type", Value: op.fstyp.String()},
		{Label: "opts", Value: op.opts},
		{Label: "state", Value: op.state.String()},
	}
}
