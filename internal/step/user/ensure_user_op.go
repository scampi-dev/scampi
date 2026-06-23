// SPDX-License-Identifier: GPL-3.0-only

package user

import (
	"context"
	"errors"
	"sort"
	"strings"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/sharedop"
	"scampi.dev/scampi/internal/target"
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

	// Home-directory ownership: the user record can match while the
	// filesystem still has /home/<name> owned by someone else (#265).
	// Common cause: a posix.dir step with `path = "/home/<name>/.ssh"`
	// runs `mkdir -p`, which silently materialises the parent as
	// root before posix.user gets a chance.
	if op.home != "" {
		homeDrift, err := op.checkHomeOwnership(ctx, tgt)
		if err != nil {
			return spec.CheckUnsatisfied, nil, err
		}
		drift = append(drift, homeDrift...)
	}

	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

// checkHomeOwnership returns a drift entry if op.home exists on the
// target with the wrong owner. Missing-home is not drift — useradd
// -m -d will materialise it on Execute. Targets that don't expose
// ownership are silently ignored (e.g. a hypothetical Windows or
// non-POSIX target — the step would have errored earlier anyway).
func (op *ensureUserOp) checkHomeOwnership(ctx context.Context, tgt target.Target) ([]spec.DriftDetail, error) {
	fsTgt, fsOk := tgt.(target.Filesystem)
	ownTgt, ownOk := tgt.(target.Ownership)
	if !fsOk || !ownOk {
		return nil, nil
	}
	if _, err := fsTgt.Stat(ctx, op.home); err != nil {
		if errors.Is(err, target.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	owner, err := ownTgt.GetOwner(ctx, op.home)
	if err != nil {
		return nil, err
	}
	if owner.User == op.name && owner.Group == op.name {
		return nil, nil
	}
	return []spec.DriftDetail{{
		Field:   "home_ownership",
		Current: owner.User + ":" + owner.Group,
		Desired: op.name + ":" + op.name,
	}}, nil
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

	changed := false

	if !exists {
		if err := um.CreateUser(ctx, info); err != nil {
			return spec.Result{}, UserCreateError{
				Name:   op.name,
				Err:    err,
				Source: op.nameSource,
			}
		}
		changed = true
	} else {
		// Check if modification is needed
		current, err := um.GetUser(ctx, op.name)
		if err != nil {
			return spec.Result{}, err
		}

		needsModify := (op.shell != "" && current.Shell != op.shell) ||
			(op.home != "" && current.Home != op.home) ||
			(op.groups != nil && !groupsEqual(current.Groups, op.groups)) ||
			op.password != ""

		if needsModify {
			if err := um.ModifyUser(ctx, info); err != nil {
				return spec.Result{}, UserModifyError{
					Name:   op.name,
					Err:    err,
					Source: op.nameSource,
				}
			}
			changed = true
		}
	}

	// Reconcile home-directory ownership if the home path is
	// managed by us and the dir exists with a different owner.
	// useradd -m -d already gets ownership right on create when
	// it materialises the home dir, but a pre-existing home (e.g.
	// from a parallel posix.dir step) won't have been touched.
	// See #265 + the bench bootstrap saga.
	if op.home != "" {
		homeChanged, err := op.reconcileHomeOwnership(ctx, tgt)
		if err != nil {
			return spec.Result{Changed: changed}, err
		}
		changed = changed || homeChanged
	}

	return spec.Result{Changed: changed}, nil
}

// reconcileHomeOwnership chowns op.home to <name>:<name> when the dir
// exists with a different owner. Returns whether the chown was
// applied. Missing dir / no-ownership-target → no-op.
func (op *ensureUserOp) reconcileHomeOwnership(ctx context.Context, tgt target.Target) (bool, error) {
	fsTgt, fsOk := tgt.(target.Filesystem)
	ownTgt, ownOk := tgt.(target.Ownership)
	if !fsOk || !ownOk {
		return false, nil
	}
	if _, err := fsTgt.Stat(ctx, op.home); err != nil {
		if errors.Is(err, target.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	desired := target.Owner{User: op.name, Group: op.name}
	owner, err := ownTgt.GetOwner(ctx, op.home)
	if err != nil {
		return false, err
	}
	if owner.User == desired.User && owner.Group == desired.Group {
		return false, nil
	}
	if err := ownTgt.Chown(ctx, op.home, desired); err != nil {
		return false, err
	}
	return true, nil
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
