// SPDX-License-Identifier: GPL-3.0-only

package container

import (
	"context"
	"sort"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const ensureContainerID = "builtin.ensure-container"

type ensureContainerOp struct {
	sharedops.BaseOp
	name    string
	image   string
	state   State
	restart string
	ports   []string
	env     map[string]string
	mounts  []target.Mount
	args    []string
	labels  map[string]string
	step    spec.StepInstance
}

func (op *ensureContainerOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cm := target.Must[target.ContainerManager](ensureContainerID, tgt)

	info, exists, err := cm.InspectContainer(ctx, op.name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, ContainerCommandError{
			Op:     "inspect",
			Name:   op.name,
			Stderr: err.Error(),
			Source: op.step.Source,
		}
	}

	// When the container will be created (or recreated), verify mount sources.
	needsCreate := !exists || (op.state != StateAbsent && len(op.configDrift(info)) > 0)
	if needsCreate && op.state != StateAbsent {
		if err := op.checkMountSources(ctx, tgt); err != nil {
			return spec.CheckUnsatisfied, nil, err
		}
	}

	switch op.state {
	case StateAbsent:
		if !exists {
			return spec.CheckSatisfied, nil, nil
		}
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Current: "present",
			Desired: "absent",
		}}, nil

	case StateStopped:
		if !exists {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "state",
				Current: "(absent)",
				Desired: "stopped",
			}}, nil
		}
		drift := op.configDrift(info)
		if info.Running {
			drift = append(drift, spec.DriftDetail{
				Field:   "state",
				Current: "running",
				Desired: "stopped",
			})
		}
		if len(drift) > 0 {
			return spec.CheckUnsatisfied, drift, nil
		}
		return spec.CheckSatisfied, nil, nil

	default: // StateRunning
		if !exists {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "state",
				Current: "(absent)",
				Desired: "running",
			}}, nil
		}
		drift := op.configDrift(info)
		if !info.Running {
			drift = append(drift, spec.DriftDetail{
				Field:   "state",
				Current: "stopped",
				Desired: "running",
			})
		}
		if len(drift) > 0 {
			return spec.CheckUnsatisfied, drift, nil
		}
		return spec.CheckSatisfied, nil, nil
	}
}

func (op *ensureContainerOp) configDrift(info target.ContainerInfo) []spec.DriftDetail {
	var drift []spec.DriftDetail

	if info.Image != op.image {
		drift = append(drift, spec.DriftDetail{
			Field:   "image",
			Current: info.Image,
			Desired: op.image,
		})
	}
	if info.Restart != op.restart {
		drift = append(drift, spec.DriftDetail{
			Field:   "restart",
			Current: info.Restart,
			Desired: op.restart,
		})
	}

	have := make([]string, len(info.Ports))
	copy(have, info.Ports)
	sort.Strings(have)
	want := make([]string, len(op.ports))
	copy(want, op.ports)
	sort.Strings(want)

	if !slicesEqual(have, want) {
		drift = append(drift, spec.DriftDetail{
			Field:   "ports",
			Current: joinOrNone(have),
			Desired: joinOrNone(want),
		})
	}

	drift = append(drift, op.envDrift(info.Env)...)
	drift = append(drift, op.mountDrift(info.Mounts)...)

	if len(op.args) > 0 && !slicesEqual(info.Args, op.args) {
		drift = append(drift, spec.DriftDetail{
			Field:   "args",
			Current: joinOrNone(info.Args),
			Desired: joinOrNone(op.args),
		})
	}

	drift = append(drift, op.labelDrift(info.Labels)...)

	return drift
}

func (op *ensureContainerOp) labelDrift(current map[string]string) []spec.DriftDetail {
	var drift []spec.DriftDetail
	for k, want := range op.labels {
		got, ok := current[k]
		if !ok {
			drift = append(drift, spec.DriftDetail{
				Field:   "label." + k,
				Current: "(unset)",
				Desired: want,
			})
		} else if got != want {
			drift = append(drift, spec.DriftDetail{
				Field:   "label." + k,
				Current: got,
				Desired: want,
			})
		}
	}
	sort.Slice(drift, func(i, j int) bool { return drift[i].Field < drift[j].Field })
	return drift
}

func (op *ensureContainerOp) mountDrift(current []target.Mount) []spec.DriftDetail {
	have := mountSet(current)
	want := mountSet(op.mounts)
	if mountsEqual(have, want) {
		return nil
	}
	return []spec.DriftDetail{{
		Field:   "mounts",
		Current: mountsStr(current),
		Desired: mountsStr(op.mounts),
	}}
}

func mountSet(mounts []target.Mount) map[target.Mount]bool {
	s := make(map[target.Mount]bool, len(mounts))
	for _, m := range mounts {
		s[m] = true
	}
	return s
}

func mountsEqual(a, b map[target.Mount]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func mountsStr(mounts []target.Mount) string {
	if len(mounts) == 0 {
		return "(none)"
	}
	strs := make([]string, len(mounts))
	for i, m := range mounts {
		strs[i] = m.String()
	}
	sort.Strings(strs)
	return strings.Join(strs, ", ")
}

func (op *ensureContainerOp) envDrift(current map[string]string) []spec.DriftDetail {
	var drift []spec.DriftDetail
	for k, want := range op.env {
		got, ok := current[k]
		if !ok {
			drift = append(drift, spec.DriftDetail{
				Field:   "env." + k,
				Current: "(unset)",
				Desired: want,
			})
		} else if got != want {
			drift = append(drift, spec.DriftDetail{
				Field:   "env." + k,
				Current: got,
				Desired: want,
			})
		}
	}
	sort.Slice(drift, func(i, j int) bool { return drift[i].Field < drift[j].Field })
	return drift
}

func (op *ensureContainerOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cm := target.Must[target.ContainerManager](ensureContainerID, tgt)

	info, exists, err := cm.InspectContainer(ctx, op.name)
	if err != nil {
		return spec.Result{}, ContainerCommandError{
			Op:     "inspect",
			Name:   op.name,
			Stderr: err.Error(),
			Source: op.step.Source,
		}
	}

	switch op.state {
	case StateAbsent:
		return op.executeAbsent(ctx, cm, info, exists)
	case StateStopped:
		return op.executeStopped(ctx, cm, info, exists)
	default:
		return op.executeRunning(ctx, cm, info, exists)
	}
}

func (op *ensureContainerOp) executeAbsent(
	ctx context.Context,
	cm target.ContainerManager,
	info target.ContainerInfo,
	exists bool,
) (spec.Result, error) {
	if !exists {
		return spec.Result{}, nil
	}
	if info.Running {
		if err := cm.StopContainer(ctx, op.name); err != nil {
			return spec.Result{}, op.cmdErr("stop", err)
		}
	}
	if err := cm.RemoveContainer(ctx, op.name); err != nil {
		return spec.Result{}, op.cmdErr("remove", err)
	}
	return spec.Result{Changed: true}, nil
}

func (op *ensureContainerOp) executeStopped(
	ctx context.Context,
	cm target.ContainerManager,
	info target.ContainerInfo,
	exists bool,
) (spec.Result, error) {
	if exists && len(op.configDrift(info)) > 0 {
		if info.Running {
			if err := cm.StopContainer(ctx, op.name); err != nil {
				return spec.Result{}, op.cmdErr("stop", err)
			}
		}
		if err := cm.RemoveContainer(ctx, op.name); err != nil {
			return spec.Result{}, op.cmdErr("remove", err)
		}
		exists = false
	}
	if !exists {
		if err := op.create(ctx, cm); err != nil {
			return spec.Result{}, err
		}
		return spec.Result{Changed: true}, nil
	}
	if info.Running {
		if err := cm.StopContainer(ctx, op.name); err != nil {
			return spec.Result{}, op.cmdErr("stop", err)
		}
		return spec.Result{Changed: true}, nil
	}
	return spec.Result{}, nil
}

func (op *ensureContainerOp) executeRunning(
	ctx context.Context,
	cm target.ContainerManager,
	info target.ContainerInfo,
	exists bool,
) (spec.Result, error) {
	if exists && len(op.configDrift(info)) > 0 {
		if info.Running {
			if err := cm.StopContainer(ctx, op.name); err != nil {
				return spec.Result{}, op.cmdErr("stop", err)
			}
		}
		if err := cm.RemoveContainer(ctx, op.name); err != nil {
			return spec.Result{}, op.cmdErr("remove", err)
		}
		exists = false
	}
	if !exists {
		if err := op.create(ctx, cm); err != nil {
			return spec.Result{}, err
		}
		if err := cm.StartContainer(ctx, op.name); err != nil {
			return spec.Result{}, op.cmdErr("start", err)
		}
		return spec.Result{Changed: true}, nil
	}
	if !info.Running {
		if err := cm.StartContainer(ctx, op.name); err != nil {
			return spec.Result{}, op.cmdErr("start", err)
		}
		return spec.Result{Changed: true}, nil
	}
	return spec.Result{}, nil
}

func (op *ensureContainerOp) create(ctx context.Context, cm target.ContainerManager) error {
	err := cm.CreateContainer(ctx, target.ContainerInfo{
		Name:    op.name,
		Image:   op.image,
		Restart: op.restart,
		Ports:   op.ports,
		Env:     op.env,
		Mounts:  op.mounts,
		Args:    op.args,
		Labels:  op.labels,
	})
	if err != nil {
		return op.cmdErr("create", err)
	}
	return nil
}

func (op *ensureContainerOp) checkMountSources(ctx context.Context, tgt target.Target) error {
	fs := target.Must[target.Filesystem](ensureContainerID, tgt)
	for _, m := range op.mounts {
		info, err := fs.Stat(ctx, m.Source)
		if err != nil {
			if target.IsNotExist(err) {
				return MountSourceMissingError{
					Path:   m.Source,
					Source: op.step.Fields["mounts"].Value,
				}
			}
			return err
		}
		if !info.IsDir() {
			return InvalidMountError{
				Got:    m.String(),
				Reason: "mount source is not a directory",
				Source: op.step.Fields["mounts"].Value,
			}
		}
	}
	return nil
}

func (op *ensureContainerOp) cmdErr(operation string, err error) ContainerCommandError {
	return ContainerCommandError{
		Op:     operation,
		Name:   op.name,
		Stderr: err.Error(),
		Source: op.step.Source,
	}
}

func (op ensureContainerOp) RequiredCapabilities() capability.Capability {
	caps := capability.Container
	if len(op.mounts) > 0 {
		caps |= capability.Filesystem
	}
	return caps
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func joinOrNone(ports []string) string {
	if len(ports) == 0 {
		return "(none)"
	}
	s := ports[0]
	for _, p := range ports[1:] {
		s += ", " + p
	}
	return s
}

// OpDescription
// -----------------------------------------------------------------------------

type ensureContainerDesc struct {
	Name  string
	State string
	Image string
}

func (d ensureContainerDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureContainerID,
		Text: `ensure container "{{.Name}}" is {{.State}} ({{.Image}})`,
		Data: d,
	}
}

func (op *ensureContainerOp) OpDescription() spec.OpDescription {
	return ensureContainerDesc{
		Name:  op.name,
		State: op.state.String(),
		Image: op.image,
	}
}
