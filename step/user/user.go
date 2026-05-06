// SPDX-License-Identifier: GPL-3.0-only

// Package user implements the user step type for managing system user accounts.
package user

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
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

// StateValues is the exhaustive list of accepted state strings.
var StateValues = []string{statePresent, stateAbsent}

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
		panic(errs.BUG("invalid user state %q — should have been caught by Validate", s))
	}
}

type (
	User       struct{}
	UserConfig struct {
		_ struct{} `summary:"Ensure a user account exists or is absent on the target"`

		Desc     string   `step:"Human-readable description" optional:"true"`
		Name     string   `step:"Username to manage" example:"hal9000"`
		State    string   `step:"Desired state" default:"present" example:"absent"`
		Shell    string   `step:"Login shell" optional:"true" example:"/bin/bash"`
		Home     string   `step:"Home directory" optional:"true" example:"/home/hal9000"`
		System   bool     `step:"Create as system user" optional:"true"`
		Password string   `step:"Password hash" optional:"true"`
		Groups   []string `step:"Supplementary groups" optional:"true" example:"[\"sudo\", \"docker\"]"`
		Promises []string `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs   []string `step:"Cross-deploy resources this step consumes" optional:"true"`
	}
	userAction struct {
		desc   string
		name   string
		state  State
		shell  string
		home   string
		system bool
		pass   string
		groups []string
		step   spec.StepInstance
	}
)

func (*UserConfig) FieldEnumValues() map[string][]string {
	return map[string][]string{
		"state": StateValues,
	}
}

func (User) Kind() string   { return "user" }
func (User) NewConfig() any { return &UserConfig{} }

func (c *UserConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (u User) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*UserConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &UserConfig{}, step.Config)
	}

	return &userAction{
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

func (a *userAction) Desc() string { return a.desc }
func (a *userAction) Kind() string { return "user" }
func (a *userAction) Inputs() []spec.Resource {
	var r []spec.Resource
	for _, g := range a.groups {
		r = append(r, spec.GroupResource(g))
	}
	return r
}

func (a *userAction) Promises() []spec.Resource {
	if a.state == StatePresent {
		return []spec.Resource{spec.UserResource(a.name)}
	}
	return nil
}

func (a *userAction) Ops() []spec.Op {
	nameSource := a.step.Fields["name"].Value

	switch a.state {
	case StateAbsent:
		op := &removeUserOp{
			name:       a.name,
			nameSource: nameSource,
		}
		op.SetAction(a)
		return []spec.Op{op}

	default:
		op := &ensureUserOp{
			name:       a.name,
			shell:      a.shell,
			home:       a.home,
			system:     a.system,
			password:   a.pass,
			groups:     a.groups,
			nameSource: nameSource,
		}
		op.SetAction(a)
		return []spec.Op{op}
	}
}
