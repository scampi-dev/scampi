// SPDX-License-Identifier: GPL-3.0-only

package user

import (
	"context"
	"sort"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const ensureUserID = "step.ensure-user"

type ensureUserOp struct {
	sharedop.BaseOp
	name       string
	shell      string
	home       string
	system     bool
	password   string
	groups     []string
	nameSource spec.SourceSpan
}

func (op *ensureUserOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	um := target.Must[target.UserManager](ensureUserID, tgt)

	exists, err := um.UserExists(ctx, op.name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	if !exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Desired: "present",
		}}, nil
	}

	info, err := um.GetUser(ctx, op.name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	var drift []spec.DriftDetail

	if op.shell != "" && info.Shell != op.shell {
		drift = append(drift, spec.DriftDetail{
			Field:   "shell",
			Current: info.Shell,
			Desired: op.shell,
		})
	}
	if op.home != "" && info.Home != op.home {
		drift = append(drift, spec.DriftDetail{
			Field:   "home",
			Current: info.Home,
			Desired: op.home,
		})
	}
	if op.groups != nil && !groupsEqual(info.Groups, op.groups) {
		drift = append(drift, spec.DriftDetail{
			Field:   "groups",
			Current: strings.Join(sorted(info.Groups), ","),
			Desired: strings.Join(sorted(op.groups), ","),
		})
	}

	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

func (op *ensureUserOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	um := target.Must[target.UserManager](ensureUserID, tgt)

	exists, err := um.UserExists(ctx, op.name)
	if err != nil {
		return spec.Result{}, err
	}

	info := target.UserInfo{
		Name:     op.name,
		Shell:    op.shell,
		Home:     op.home,
		System:   op.system,
		Password: op.password,
		Groups:   op.groups,
	}

	if !exists {
		if err := um.CreateUser(ctx, info); err != nil {
			return spec.Result{}, UserCreateError{
				Name:   op.name,
				Err:    err,
				Source: op.nameSource,
			}
		}
		return spec.Result{Changed: true}, nil
	}

	// Check if modification is needed
	current, err := um.GetUser(ctx, op.name)
	if err != nil {
		return spec.Result{}, err
	}

	needsModify := (op.shell != "" && current.Shell != op.shell) ||
		(op.home != "" && current.Home != op.home) ||
		(op.groups != nil && !groupsEqual(current.Groups, op.groups)) ||
		op.password != ""

	if !needsModify {
		return spec.Result{Changed: false}, nil
	}

	if err := um.ModifyUser(ctx, info); err != nil {
		return spec.Result{}, UserModifyError{
			Name:   op.name,
			Err:    err,
			Source: op.nameSource,
		}
	}
	return spec.Result{Changed: true}, nil
}

func (ensureUserOp) RequiredCapabilities() capability.Capability {
	return capability.User
}

type ensureUserDesc struct {
	Name string
}

func (d ensureUserDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureUserID,
		Text: `ensure user "{{.Name}}" is present`,
		Data: d,
	}
}

func (op *ensureUserOp) OpDescription() spec.OpDescription {
	return ensureUserDesc{Name: op.name}
}

func (op *ensureUserOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "name", Value: op.name},
	}
	if op.shell != "" {
		fields = append(fields, spec.InspectField{Label: "shell", Value: op.shell})
	}
	if op.home != "" {
		fields = append(fields, spec.InspectField{Label: "home", Value: op.home})
	}
	if len(op.groups) > 0 {
		fields = append(fields, spec.InspectField{Label: "groups", Value: strings.Join(op.groups, ", ")})
	}
	if op.system {
		fields = append(fields, spec.InspectField{Label: "system", Value: "true"})
	}
	return fields
}

func groupsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := sorted(a)
	sb := sorted(b)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

func sorted(s []string) []string {
	cp := make([]string, len(s))
	copy(cp, s)
	sort.Strings(cp)
	return cp
}
