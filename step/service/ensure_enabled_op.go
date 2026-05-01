// SPDX-License-Identifier: GPL-3.0-only

package service

import (
	"context"
	"fmt"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const ensureEnabledID = "step.ensure-service-enabled"

type ensureEnabledOp struct {
	sharedop.BaseOp
	name       string
	enabled    bool
	nameSource spec.SourceSpan
}

func (op *ensureEnabledOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	sm := target.Must[target.ServiceManager](ensureEnabledID, tgt)

	enabled, err := sm.IsEnabled(ctx, op.name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	if enabled == op.enabled {
		return spec.CheckSatisfied, nil, nil
	}

	current := "disabled"
	if enabled {
		current = "enabled"
	}
	desired := "disabled"
	if op.enabled {
		desired = "enabled"
	}
	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "enabled",
		Current: current,
		Desired: desired,
	}}, nil
}

func (op *ensureEnabledOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	sm := target.Must[target.ServiceManager](ensureEnabledID, tgt)

	enabled, err := sm.IsEnabled(ctx, op.name)
	if err != nil {
		return spec.Result{}, err
	}

	if enabled == op.enabled {
		return spec.Result{Changed: false}, nil
	}

	if op.enabled {
		if err := sm.Enable(ctx, op.name); err != nil {
			return spec.Result{}, ServiceCommandError{
				Op:     "enable",
				Name:   op.name,
				Stderr: err.Error(),
				Source: op.nameSource,
			}
		}
	} else {
		if err := sm.Disable(ctx, op.name); err != nil {
			return spec.Result{}, ServiceCommandError{
				Op:     "disable",
				Name:   op.name,
				Stderr: err.Error(),
				Source: op.nameSource,
			}
		}
	}

	return spec.Result{Changed: true}, nil
}

func (ensureEnabledOp) RequiredCapabilities() capability.Capability {
	return capability.Service
}

type ensureEnabledDesc struct {
	Name    string
	Enabled string
}

func (d ensureEnabledDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureEnabledID,
		Text: `ensure service {{.Name}} is {{.Enabled}}`,
		Data: d,
	}
}

func (op *ensureEnabledOp) OpDescription() spec.OpDescription {
	return ensureEnabledDesc{
		Name:    op.name,
		Enabled: fmt.Sprintf("%v", op.enabled),
	}
}

func (op *ensureEnabledOp) Inspect() []spec.InspectField {
	v := "false"
	if op.enabled {
		v = "true"
	}
	return []spec.InspectField{
		{Label: "name", Value: op.name},
		{Label: "enabled", Value: v},
	}
}
