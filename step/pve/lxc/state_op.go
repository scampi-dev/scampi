// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const stateLxcID = "step.pve.lxc.state"

type stateLxcOp struct {
	sharedops.BaseOp
	pveCmd
	state State
}

func (op *stateLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](stateLxcID, tgt)

	exists, status, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if !exists {
		// Container doesn't exist yet — createLxcOp will handle it.
		// Return unsatisfied so Execute runs after create.
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Current: "(pending create)",
			Desired: op.state.String(),
		}}, nil
	}

	switch op.state {
	case StateRunning:
		if status != stateRunning {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "state",
				Current: status,
				Desired: stateRunning,
			}}, nil
		}
	case StateStopped:
		if status != stateStopped {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "state",
				Current: status,
				Desired: stateStopped,
			}}, nil
		}
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *stateLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](stateLxcID, tgt)

	exists, status, err := op.inspectExists(ctx, cmdr)
	if err != nil || !exists {
		return spec.Result{}, err
	}

	switch op.state {
	case StateRunning:
		if status != stateRunning {
			if err := op.runCmd(ctx, cmdr, "start", fmt.Sprintf("pct start %d", op.id)); err != nil {
				return spec.Result{}, err
			}
			return spec.Result{Changed: true}, nil
		}
	case StateStopped:
		if status != stateStopped {
			if err := op.runCmd(ctx, cmdr, "shutdown", fmt.Sprintf("pct shutdown %d --timeout 30", op.id)); err != nil {
				return spec.Result{}, err
			}
			return spec.Result{Changed: true}, nil
		}
	}

	return spec.Result{}, nil
}

func (stateLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type stateLxcDesc struct {
	VMID  int
	State string
}

func (d stateLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   stateLxcID,
		Text: `ensure LXC {{.VMID}} is {{.State}}`,
		Data: d,
	}
}

func (op *stateLxcOp) OpDescription() spec.OpDescription {
	return stateLxcDesc{
		VMID:  op.id,
		State: op.state.String(),
	}
}

func (op *stateLxcOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "vmid", Value: fmt.Sprintf("%d", op.id)},
		{Label: "state", Value: op.state.String()},
	}
}
