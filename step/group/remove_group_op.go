// SPDX-License-Identifier: GPL-3.0-only

package group

import (
	"context"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const removeGroupID = "builtin.remove-group"

type removeGroupOp struct {
	sharedops.BaseOp
	name       string
	nameSource spec.SourceSpan
}

func (op *removeGroupOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	gm := target.Must[target.GroupManager](removeGroupID, tgt)

	exists, err := gm.GroupExists(ctx, op.name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	if exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Current: "present",
			Desired: "absent",
		}}, nil
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *removeGroupOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	gm := target.Must[target.GroupManager](removeGroupID, tgt)

	exists, err := gm.GroupExists(ctx, op.name)
	if err != nil {
		return spec.Result{}, err
	}

	if !exists {
		return spec.Result{Changed: false}, nil
	}

	if err := gm.DeleteGroup(ctx, op.name); err != nil {
		return spec.Result{}, GroupDeleteError{
			Name:   op.name,
			Err:    err,
			Source: op.nameSource,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (removeGroupOp) RequiredCapabilities() capability.Capability {
	return capability.Group
}

type removeGroupDesc struct {
	Name string
}

func (d removeGroupDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   removeGroupID,
		Text: `ensure group "{{.Name}}" is absent`,
		Data: d,
	}
}

func (op *removeGroupOp) OpDescription() spec.OpDescription {
	return removeGroupDesc{Name: op.name}
}

func (op *removeGroupOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "name", Value: op.name},
		{Label: "state", Value: "absent"},
	}
}
