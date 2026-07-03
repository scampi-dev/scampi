// SPDX-License-Identifier: GPL-3.0-only

// Package group implements the group step type for managing system groups.
package group

import (
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

// State represents the desired group state.
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
		panic(errs.BUG("invalid group state %q - should have been caught by Validate", s))
	}
}

type (
	Group       struct{}
	GroupConfig struct {
		Desc     string
		Name     string
		State    string
		GID      int
		System   bool
		Provides []string
		Requires []string
	}
	groupStep struct {
		desc   string
		name   string
		state  State
		gid    int
		system bool
		step   spec.DeclaredStep
	}
)

func (Group) Kind() string   { return "group" }
func (Group) NewConfig() any { return &GroupConfig{} }

func (c *GroupConfig) ResourceDeclarations() (provides, requires []string) {
	return c.Provides, c.Requires
}

func (g Group) Plan(step spec.DeclaredStep) (spec.Step, error) {
	cfg, ok := step.Config.(*GroupConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &GroupConfig{}, step.Config)
	}

	return &groupStep{
		desc:   cfg.Desc,
		name:   cfg.Name,
		state:  parseState(cfg.State),
		gid:    cfg.GID,
		system: cfg.System,
		step:   step,
	}, nil
}

func (a *groupStep) Desc() string { return a.desc }
func (a *groupStep) Kind() string { return "group" }

func (a *groupStep) Provides() []spec.Resource {
	if a.state == StatePresent {
		return []spec.Resource{spec.GroupResource(a.name)}
	}
	return nil
}

func (a *groupStep) Ops() []spec.Op {
	nameSource := a.step.Fields["name"].Value

	switch a.state {
	case StateAbsent:
		op := &removeGroupOp{
			name:     a.name,
			nameSpan: nameSource,
		}
		op.SetStep(a)
		return []spec.Op{op}

	default:
		op := &ensureGroupOp{
			name:     a.name,
			gid:      a.gid,
			system:   a.system,
			nameSpan: nameSource,
		}
		op.SetStep(a)
		return []spec.Op{op}
	}
}
