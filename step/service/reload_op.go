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

const reloadID = "builtin.reload-service"

type reloadOp struct {
	sharedops.BaseOp
	name              string
	nameSource        spec.SourceSpan
	fallbackToRestart bool
}

func (op *reloadOp) Check(
	_ context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	sm := target.Must[target.ServiceManager](reloadID, tgt)

	desired := StateReloaded.String()
	if !sm.SupportsReload() {
		op.fallbackToRestart = true
		desired = "restarted (reload not supported)"
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "state",
		Desired: desired,
	}}, nil
}

func (op *reloadOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	sm := target.Must[target.ServiceManager](reloadID, tgt)

	if err := sm.DaemonReload(ctx); err != nil {
		return spec.Result{}, DaemonReloadError{
			Name:   op.name,
			Stderr: err.Error(),
			Source: op.nameSource,
		}
	}

	if op.fallbackToRestart {
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

	if err := sm.Reload(ctx, op.name); err != nil {
		return spec.Result{}, ServiceCommandError{
			Op:     "reload",
			Name:   op.name,
			Stderr: err.Error(),
			Source: op.nameSource,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (reloadOp) RequiredCapabilities() capability.Capability {
	return capability.Service
}

type reloadDesc struct {
	Name string
}

func (d reloadDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   reloadID,
		Text: `reload service {{.Name}}`,
		Data: d,
	}
}

func (op *reloadOp) OpDescription() spec.OpDescription {
	return reloadDesc{Name: op.name}
}
