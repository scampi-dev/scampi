// SPDX-License-Identifier: GPL-3.0-only

package service

import (
	"context"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/controller"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/sharedop"
	"scampi.dev/scampi/internal/target"
)

const reloadID = "reload_service"

type reloadOp struct {
	sharedop.BaseOp
	name     string
	nameSpan spec.Span
}

func (op *reloadOp) Check(
	_ context.Context,
	_ controller.Controller,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	sm := target.Must[target.ServiceManager](reloadID, tgt)

	desired := StateReloaded.String()
	if !sm.SupportsReload() {
		desired = "restarted (reload not supported)"
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "state",
		Desired: desired,
	}}, nil
}

func (op *reloadOp) Execute(
	ctx context.Context,
	_ controller.Controller,
	tgt target.Target,
) (spec.Result, error) {
	sm := target.Must[target.ServiceManager](reloadID, tgt)

	if err := sm.DaemonReload(ctx); err != nil {
		return spec.Result{}, DaemonReloadError{
			Name:   op.name,
			Stderr: err.Error(),
			Span:   op.nameSpan,
		}
	}

	if !sm.SupportsReload() {
		if err := sm.Restart(ctx, op.name); err != nil {
			return spec.Result{}, ServiceCommandError{
				Op:     "restart",
				Name:   op.name,
				Stderr: err.Error(),
				Span:   op.nameSpan,
			}
		}
		return spec.Result{Changed: true}, nil
	}

	if err := sm.Reload(ctx, op.name); err != nil {
		return spec.Result{}, ServiceCommandError{
			Op:     "reload",
			Name:   op.name,
			Stderr: err.Error(),
			Span:   op.nameSpan,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (reloadOp) RequiredCapabilities() capability.Capability {
	return capability.Service
}

func (op *reloadOp) Describe() spec.OpDescription {
	return spec.OpDescription{
		ID:   reloadID,
		Text: `reload service {{.Name}}`,
		Data: struct {
			Name string
		}{
			Name: op.name,
		},
	}
}
