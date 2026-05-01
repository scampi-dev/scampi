// SPDX-License-Identifier: GPL-3.0-only

package sysctl

import (
	"context"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const cleanupSysctlID = "step.sysctl.cleanup"

type cleanupSysctlOp struct {
	sharedop.BaseOp
	path string
}

func (op *cleanupSysctlOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fs := target.Must[target.Filesystem](cleanupSysctlID, tgt)

	_, err := fs.ReadFile(ctx, op.path)
	if err != nil {
		if target.IsNotExist(err) {
			return spec.CheckSatisfied, nil, nil
		}
		return spec.CheckUnsatisfied, nil, sharedop.DiagnoseTargetError(err)
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "drop-in",
		Current: op.path,
		Desired: "(absent)",
	}}, nil
}

func (op *cleanupSysctlOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	fs := target.Must[target.Filesystem](cleanupSysctlID, tgt)

	if err := fs.Remove(ctx, op.path); err != nil {
		if target.IsNotExist(err) {
			return spec.Result{}, nil
		}
		return spec.Result{}, CleanupError{
			Path: op.path,
			Err:  sharedop.DiagnoseTargetError(err),
		}
	}

	return spec.Result{Changed: true}, nil
}

func (cleanupSysctlOp) RequiredCapabilities() capability.Capability {
	return capability.Filesystem
}

// OpDescription
// -----------------------------------------------------------------------------

type cleanupSysctlDesc struct {
	Path string
}

func (d cleanupSysctlDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   cleanupSysctlID,
		Text: `remove stale drop-in {{.Path}}`,
		Data: d,
	}
}

func (op *cleanupSysctlOp) OpDescription() spec.OpDescription {
	return cleanupSysctlDesc{Path: op.path}
}
