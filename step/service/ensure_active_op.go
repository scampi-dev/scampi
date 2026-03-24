// SPDX-License-Identifier: GPL-3.0-only

package service

import (
	"context"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const ensureActiveID = "builtin.ensure-service-active"

type ensureActiveOp struct {
	sharedops.BaseOp
	name       string
	state      State
	nameSource spec.SourceSpan
}

func (op *ensureActiveOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	sm := target.Must[target.ServiceManager](ensureActiveID, tgt)

	active, err := sm.IsActive(ctx, op.name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	wantRunning := op.state == StateRunning
	if active == wantRunning {
		return spec.CheckSatisfied, nil, nil
	}

	current := "stopped"
	if active {
		current = "running"
	}
	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "state",
		Current: current,
		Desired: op.state.String(),
	}}, nil
}

func (op *ensureActiveOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	sm := target.Must[target.ServiceManager](ensureActiveID, tgt)

	active, err := sm.IsActive(ctx, op.name)
	if err != nil {
		return spec.Result{}, err
	}

	wantRunning := op.state == StateRunning
	if active == wantRunning {
		return spec.Result{Changed: false}, nil
	}

	if wantRunning {
		if err := sm.DaemonReload(ctx); err != nil {
			return spec.Result{}, DaemonReloadError{
				Name:   op.name,
				Stderr: err.Error(),
				Source: op.nameSource,
			}
		}
		if err := sm.Start(ctx, op.name); err != nil {
			return spec.Result{}, ServiceCommandError{
				Op:     "start",
				Name:   op.name,
				Stderr: err.Error(),
				Source: op.nameSource,
			}
		}
	} else {
		if err := sm.Stop(ctx, op.name); err != nil {
			return spec.Result{}, ServiceCommandError{
				Op:     "stop",
				Name:   op.name,
				Stderr: err.Error(),
				Source: op.nameSource,
			}
		}
	}

	return spec.Result{Changed: true}, nil
}

func (ensureActiveOp) RequiredCapabilities() capability.Capability {
	return capability.Service
}

type ensureActiveDesc struct {
	Name  string
	State State
}

func (d ensureActiveDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureActiveID,
		Text: `ensure service {{.Name}} is {{.State}}`,
		Data: d,
	}
}

func (op *ensureActiveOp) OpDescription() spec.OpDescription {
	return ensureActiveDesc{
		Name:  op.name,
		State: op.state,
	}
}

func (op *ensureActiveOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "name", Value: op.name},
		{Label: "state", Value: op.state.String()},
	}
}
