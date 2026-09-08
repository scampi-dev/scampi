// SPDX-License-Identifier: GPL-3.0-only

// Package user implements the user step type for managing system user accounts.
package user

import (
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

// State represents the desired user state.
type State uint8

const (
	StatePresent State = iota + 1
	StateAbsent
)

const (
	statePresent = "present"
	stateAbsent  = "absent"
)

func (s State) String() string {
	switch s {
	case StatePresent:
		return statePresent
	case StateAbsent:
		return stateAbsent
	default:
		return "unknown"
	}
}

func parseState(s string) State {
	switch s {
	case statePresent:
		return StatePresent
	case stateAbsent:
		return StateAbsent
	default:
		panic(errs.BUG("invalid user state %q - should have been caught by Validate", s))
	}
}

type (
	User       struct{}
	UserConfig struct {
		Desc     string
		Name     string
		State    string
		Shell    string
		Home     string
		System   bool
		Password string
		Groups   []string
		Provides []string
		Requires []string
	}
	userStep struct {
		desc   string
		name   string
		state  State
		shell  string
		home   string
		system bool
		pass   string
		groups []string
		step   spec.DeclaredStep
	}
)

func (User) Kind() string   { return "user" }
func (User) NewConfig() any { return &UserConfig{} }

func (c *UserConfig) ResourceDeclarations() (provides, requires []string) {
	return c.Provides, c.Requires
}

func (u User) Plan(step spec.DeclaredStep) (spec.Step, error) {
	cfg, ok := step.Config.(*UserConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &UserConfig{}, step.Config)
	}

	return &userStep{
		desc:   cfg.Desc,
		name:   cfg.Name,
		state:  parseState(cfg.State),
		shell:  cfg.Shell,
		home:   cfg.Home,
		system: cfg.System,
		pass:   cfg.Password,
		groups: cfg.Groups,
		step:   step,
	}, nil
}

func (a *userStep) Desc() string { return a.desc }
func (a *userStep) Kind() string { return "user" }
func (a *userStep) Requires() []spec.Resource {
	var r []spec.Resource
	for _, g := range a.groups {
		r = append(r, spec.GroupResource(g))
	}
	return r
}

func (a *userStep) Provides() []spec.Resource {
	if a.state == StatePresent {
		return []spec.Resource{spec.UserResource(a.name)}
	}
	return nil
}

func (a *userStep) Ops() []spec.Op {
	nameSpan := a.step.Fields["name"].Value

	switch a.state {
	case StateAbsent:
		op := &removeUserOp{
			name:     a.name,
			nameSpan: nameSpan,
		}
		op.SetStep(a)
		return []spec.Op{op}

	default:
		op := &ensureUserOp{
			name:     a.name,
			shell:    a.shell,
			home:     a.home,
			system:   a.system,
			password: a.pass,
			groups:   a.groups,
			nameSpan: nameSpan,
		}
		op.SetStep(a)
		return []spec.Op{op}
	}
}
