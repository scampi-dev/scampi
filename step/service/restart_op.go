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

const restartID = "step.restart-service"

type restartOp struct {
	sharedops.BaseOp
	name       string
	nameSource spec.SourceSpan
}

func (op *restartOp) Check(
	_ context.Context,
	_ source.Source,
	_ target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "state",
		Desired: StateRestarted.String(),
	}}, nil
}

func (op *restartOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	sm := target.Must[target.ServiceManager](restartID, tgt)

	if err := sm.DaemonReload(ctx); err != nil {
		return spec.Result{}, DaemonReloadError{
			Name:   op.name,
			Stderr: err.Error(),
			Source: op.nameSource,
		}
	}

	if err := sm.Restart(ctx, op.name); err != nil {
		return spec.Result{}, ServiceCommandError{
			Op:     "restart",
			Name:   op.name,
			Stderr: err.Error(),
			Source: op.nameSource,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (restartOp) RequiredCapabilities() capability.Capability {
	return capability.Service
}

type restartDesc struct {
	Name string
}

func (d restartDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   restartID,
		Text: `restart service {{.Name}}`,
		Data: d,
	}
}

func (op *restartOp) OpDescription() spec.OpDescription {
	return restartDesc{Name: op.name}
}
