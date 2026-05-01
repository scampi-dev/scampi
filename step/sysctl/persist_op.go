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

const persistSysctlID = "step.sysctl.persist"

type persistSysctlOp struct {
	sharedop.BaseOp
	key   string
	value string
	path  string
}

func (op *persistSysctlOp) desiredContent() []byte {
	return []byte(op.key + " = " + op.value + "\n")
}

func (op *persistSysctlOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fs := target.Must[target.Filesystem](persistSysctlID, tgt)

	current, err := fs.ReadFile(ctx, op.path)
	if err != nil {
		if target.IsNotExist(err) {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "drop-in",
				Current: "(absent)",
				Desired: op.path,
			}}, nil
		}
		return spec.CheckUnsatisfied, nil, sharedop.DiagnoseTargetError(err)
	}

	desired := op.desiredContent()
	if string(current) == string(desired) {
		return spec.CheckSatisfied, nil, nil
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "drop-in",
		Current: string(current),
		Desired: string(desired),
	}}, nil
}

func (op *persistSysctlOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	fs := target.Must[target.Filesystem](persistSysctlID, tgt)

	if err := fs.WriteFile(ctx, op.path, op.desiredContent()); err != nil {
		return spec.Result{}, PersistError{
			Path: op.path,
			Err:  sharedop.DiagnoseTargetError(err),
		}
	}

	return spec.Result{Changed: true}, nil
}

func (persistSysctlOp) RequiredCapabilities() capability.Capability {
	return capability.Filesystem
}

// OpDescription
// -----------------------------------------------------------------------------

type persistSysctlDesc struct {
	Path string
}

func (d persistSysctlDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   persistSysctlID,
		Text: `persist sysctl to {{.Path}}`,
		Data: d,
	}
}

func (op *persistSysctlOp) OpDescription() spec.OpDescription {
	return persistSysctlDesc{Path: op.path}
}
