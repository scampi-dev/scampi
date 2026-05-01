// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const sshKeysLxcID = "step.pve.lxc.ssh-keys"

type sshKeysLxcOp struct {
	sharedop.BaseOp
	pveCmd
	sshPublicKeys []string
}

func (op *sshKeysLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](sshKeysLxcID, tgt)

	exists, status, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if !exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "ssh_public_keys",
			Current: "(pending create)",
			Desired: fmt.Sprintf("%d key(s)", len(op.sshPublicKeys)),
		}}, nil
	}
	if status != stateRunning {
		if len(op.sshPublicKeys) > 0 {
			return spec.CheckSatisfied, nil, SSHKeysSkippedWarning{
				VMID:   op.id,
				Source: op.step.Source,
			}
		}
		return spec.CheckSatisfied, nil, nil
	}

	d := op.sshKeyDrift(ctx, cmdr)
	if d == nil {
		return spec.CheckSatisfied, nil, nil
	}
	return spec.CheckUnsatisfied, []spec.DriftDetail{*d}, nil
}

func (op *sshKeysLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](sshKeysLxcID, tgt)

	exists, status, err := op.inspectExists(ctx, cmdr)
	if err != nil || !exists || status != stateRunning {
		return spec.Result{}, err
	}

	d := op.sshKeyDrift(ctx, cmdr)
	if d == nil {
		return spec.Result{}, nil
	}

	if err := op.applySSHKeys(ctx, cmdr, tgt); err != nil {
		return spec.Result{}, err
	}
	return spec.Result{Changed: true}, nil
}

func (op *sshKeysLxcOp) sshKeyDrift(ctx context.Context, cmdr target.Command) *spec.DriftDetail {
	// Check if the file exists before pulling — avoids PVE task log errors.
	testCmd := fmt.Sprintf("pct exec %d -- test -f /root/.ssh/authorized_keys", op.id)
	result, err := cmdr.RunPrivileged(ctx, testCmd)
	if err != nil || result.ExitCode != 0 {
		// File doesn't exist.
		if len(op.sshPublicKeys) == 0 {
			return nil
		}
		return &spec.DriftDetail{
			Field:   "ssh_public_keys",
			Current: "0 key(s)",
			Desired: fmt.Sprintf("%d key(s)", len(op.sshPublicKeys)),
		}
	}

	// File exists — pull and compare.
	tmpFile := fmt.Sprintf("/tmp/.scampi-ssh-read-%d", op.id)
	pullCmd := fmt.Sprintf("pct pull %d /root/.ssh/authorized_keys %s", op.id, shellQuote(tmpFile))
	result, err = cmdr.RunPrivileged(ctx, pullCmd)
	if err != nil || result.ExitCode != 0 {
		if len(op.sshPublicKeys) == 0 {
			return nil
		}
		return &spec.DriftDetail{
			Field:   "ssh_public_keys",
			Current: "0 key(s)",
			Desired: fmt.Sprintf("%d key(s)", len(op.sshPublicKeys)),
		}
	}

	// Read the pulled file, then clean up.
	result, err = cmdr.RunCommand(ctx, "cat "+shellQuote(tmpFile))
	_, _ = cmdr.RunCommand(ctx, "rm -f "+shellQuote(tmpFile))
	if err != nil || result.ExitCode != 0 {
		if len(op.sshPublicKeys) == 0 {
			return nil
		}
		return &spec.DriftDetail{
			Field:   "ssh_public_keys",
			Current: "0 key(s)",
			Desired: fmt.Sprintf("%d key(s)", len(op.sshPublicKeys)),
		}
	}

	current := parsePVEKeys(result.Stdout)
	if slicesEqual(current, op.sshPublicKeys) {
		return nil
	}

	return &spec.DriftDetail{
		Field:   "ssh_public_keys",
		Current: fmt.Sprintf("%d key(s)", len(current)),
		Desired: fmt.Sprintf("%d key(s)", len(op.sshPublicKeys)),
	}
}

func (op *sshKeysLxcOp) applySSHKeys(ctx context.Context, cmdr target.Command, tgt target.Target) error {
	content := buildAuthorizedKeys(op.sshPublicKeys)
	tmpFile := fmt.Sprintf("/tmp/.scampi-ssh-keys-%d", op.id)

	fs := target.Must[target.Filesystem](sshKeysLxcID, tgt)
	if err := fs.WriteFile(ctx, tmpFile, []byte(content)); err != nil {
		return op.cmdErr("write ssh keys", err.Error())
	}
	defer func() { _ = fs.Remove(ctx, tmpFile) }()

	mkdirCmd := fmt.Sprintf("pct exec %d -- mkdir -p /root/.ssh", op.id)
	if err := op.runCmd(ctx, cmdr, "ssh key setup", mkdirCmd); err != nil {
		return err
	}
	pushCmd := fmt.Sprintf("pct push %d %s /root/.ssh/authorized_keys --perms 600", op.id, shellQuote(tmpFile))
	if err := op.runCmd(ctx, cmdr, "push ssh keys", pushCmd); err != nil {
		return err
	}
	return nil
}

func (sshKeysLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command | capability.Filesystem
}

// OpDescription
// -----------------------------------------------------------------------------

type sshKeysLxcDesc struct {
	VMID int
	Keys int
}

func (d sshKeysLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   sshKeysLxcID,
		Text: `manage SSH keys for LXC {{.VMID}} ({{.Keys}} key(s))`,
		Data: d,
	}
}

func (op *sshKeysLxcOp) OpDescription() spec.OpDescription {
	return sshKeysLxcDesc{
		VMID: op.id,
		Keys: len(op.sshPublicKeys),
	}
}

func (op *sshKeysLxcOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "vmid", Value: fmt.Sprintf("%d", op.id)},
		{Label: "keys", Value: fmt.Sprintf("%d", len(op.sshPublicKeys))},
	}
}
