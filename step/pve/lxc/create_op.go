// SPDX-License-Identifier: GPL-3.0-only

package lxc

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

const createLxcID = "step.pve.lxc.create"

type createLxcOp struct {
	sharedops.BaseOp
	pveCmd
	template      *LxcTemplate
	hostname      string
	state         State
	cpu           LxcCPU
	memoryMiB     int
	swapMiB       int
	storage       string
	sizeGiB       int
	privileged    bool
	networks      []LxcNet
	devices       []LxcDevice
	tags          []string
	sshPublicKeys []string
}

func (op *createLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](createLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	if op.state == StateAbsent {
		if !exists {
			return spec.CheckSatisfied, nil, nil
		}
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Current: "present",
			Desired: stateAbsent,
		}}, nil
	}

	if !exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Current: "(absent)",
			Desired: "present",
		}}, nil
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *createLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](createLxcID, tgt)

	exists, status, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	if op.state == StateAbsent {
		if !exists {
			return spec.Result{}, nil
		}
		if status == stateRunning {
			if err := op.runCmd(ctx, cmdr, "shutdown", fmt.Sprintf("pct shutdown %d --timeout 30", op.id)); err != nil {
				return spec.Result{}, err
			}
		}
		if err := op.runCmd(ctx, cmdr, "destroy", fmt.Sprintf("pct destroy %d --purge", op.id)); err != nil {
			return spec.Result{}, err
		}
		return spec.Result{Changed: true}, nil
	}

	if exists {
		return spec.Result{}, nil
	}

	cfg := *op.Action().(*lxcAction)
	createCmd := buildCreateCmd(cfg)

	// SSH keys: write to temp file, pass to pct create, clean up after.
	var keyFile string
	if len(op.sshPublicKeys) > 0 {
		keyFile = fmt.Sprintf("/tmp/.scampi-ssh-keys-%d", op.id)
		fs := target.Must[target.Filesystem](createLxcID, tgt)
		keyContent := strings.Join(op.sshPublicKeys, "\n") + "\n"
		if err := fs.WriteFile(ctx, keyFile, []byte(keyContent)); err != nil {
			return spec.Result{}, op.cmdErr("write ssh keys", err.Error())
		}
		createCmd += " --ssh-public-keys " + shellQuote(keyFile)
	}

	err = op.runCmd(ctx, cmdr, "create", createCmd)

	if keyFile != "" {
		fs := target.Must[target.Filesystem](createLxcID, tgt)
		_ = fs.Remove(ctx, keyFile)
	}

	if err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func (createLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command | capability.Filesystem
}

// OpDescription
// -----------------------------------------------------------------------------

type createLxcDesc struct {
	VMID     int
	Hostname string
	State    string
}

func (d createLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   createLxcID,
		Text: `ensure LXC {{.VMID}} "{{.Hostname}}" exists ({{.State}})`,
		Data: d,
	}
}

func (op *createLxcOp) OpDescription() spec.OpDescription {
	s := "create"
	if op.state == StateAbsent {
		s = "destroy"
	}
	return createLxcDesc{
		VMID:     op.id,
		Hostname: op.hostname,
		State:    s,
	}
}

func (op *createLxcOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "vmid", Value: fmt.Sprintf("%d", op.id)},
		{Label: "hostname", Value: op.hostname},
		{Label: "template", Value: templateStr(op.template)},
	}
}
