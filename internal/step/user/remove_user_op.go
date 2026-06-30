// SPDX-License-Identifier: GPL-3.0-only

package user

import (
	"context"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/sharedop"
	"scampi.dev/scampi/internal/target"
)

const removeUserID = "remove_user"

type removeUserOp struct {
	sharedop.BaseOp
	name       string
	nameSource spec.SourceSpan
}

func (op *removeUserOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	um := target.Must[target.UserManager](removeUserID, tgt)

	exists, err := um.UserExists(ctx, op.name)
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

func (op *removeUserOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	um := target.Must[target.UserManager](removeUserID, tgt)

	exists, err := um.UserExists(ctx, op.name)
	if err != nil {
		return spec.Result{}, err
	}

	if !exists {
		return spec.Result{Changed: false}, nil
	}

	if err := um.DeleteUser(ctx, op.name); err != nil {
		return spec.Result{}, UserDeleteError{
			Name:   op.name,
			Err:    err,
			Source: op.nameSource,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (removeUserOp) RequiredCapabilities() capability.Capability {
	return capability.User
}

type removeUserDesc struct {
	Name string
}

func (d removeUserDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   removeUserID,
		Text: `ensure user "{{.Name}}" is absent`,
		Data: d,
	}
}

func (op *removeUserOp) OpDescription() spec.OpDescription {
	return removeUserDesc{Name: op.name}
}

func (op *removeUserOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "name", Value: op.name},
		{Label: "state", Value: "absent"},
	}
}
