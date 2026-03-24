// SPDX-License-Identifier: GPL-3.0-only

package group

import (
	"context"
	"strconv"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const ensureGroupID = "builtin.ensure-group"

type ensureGroupOp struct {
	sharedops.BaseOp
	name       string
	gid        int
	system     bool
	nameSource spec.SourceSpan
}

func (op *ensureGroupOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	gm := target.Must[target.GroupManager](ensureGroupID, tgt)

	exists, err := gm.GroupExists(ctx, op.name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	if !exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Desired: "present",
		}}, nil
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *ensureGroupOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	gm := target.Must[target.GroupManager](ensureGroupID, tgt)

	exists, err := gm.GroupExists(ctx, op.name)
	if err != nil {
		return spec.Result{}, err
	}

	if exists {
		return spec.Result{Changed: false}, nil
	}

	info := target.GroupInfo{
		Name:   op.name,
		GID:    op.gid,
		System: op.system,
	}

	if err := gm.CreateGroup(ctx, info); err != nil {
		return spec.Result{}, GroupCreateError{
			Name:   op.name,
			Err:    err,
			Source: op.nameSource,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (ensureGroupOp) RequiredCapabilities() capability.Capability {
	return capability.Group
}

type ensureGroupDesc struct {
	Name string
}

func (d ensureGroupDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureGroupID,
		Text: `ensure group "{{.Name}}" is present`,
		Data: d,
	}
}

func (op *ensureGroupOp) OpDescription() spec.OpDescription {
	return ensureGroupDesc{Name: op.name}
}

func (op *ensureGroupOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "name", Value: op.name},
	}
	if op.gid > 0 {
		fields = append(fields, spec.InspectField{Label: "gid", Value: strconv.Itoa(op.gid)})
	}
	if op.system {
		fields = append(fields, spec.InspectField{Label: "system", Value: "true"})
	}
	return fields
}
