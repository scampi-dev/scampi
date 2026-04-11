// SPDX-License-Identifier: GPL-3.0-only

// Package group implements the group step type for managing system groups.
package group

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
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
		panic(errs.BUG("invalid group state %q — should have been caught by Validate", s))
	}
}

type (
	Group       struct{}
	GroupConfig struct {
		_ struct{} `summary:"Ensure a group exists or is absent on the target"`

		Desc   string `step:"Human-readable description" optional:"true"`
		Name   string `step:"Group name to manage" example:"appusers"`
		State  string `step:"Desired state" default:"present" example:"absent"`
		GID    int    `step:"Group ID" optional:"true" example:"1100"`
		System bool   `step:"Create as system group" optional:"true"`
	}
	groupAction struct {
		desc   string
		name   string
		state  State
		gid    int
		system bool
		step   spec.StepInstance
	}
)

func (*GroupConfig) FieldEnumValues() map[string][]string {
	return map[string][]string{
		"state": StateValues,
	}
}

func (Group) Kind() string   { return "group" }
func (Group) NewConfig() any { return &GroupConfig{} }

func (g Group) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*GroupConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &GroupConfig{}, step.Config)
	}

	return &groupAction{
		desc:   cfg.Desc,
		name:   cfg.Name,
		state:  parseState(cfg.State),
		gid:    cfg.GID,
		system: cfg.System,
		step:   step,
	}, nil
}

func (a *groupAction) Desc() string            { return a.desc }
func (a *groupAction) Kind() string            { return "group" }
func (a *groupAction) Inputs() []spec.Resource { return nil }

func (a *groupAction) Promises() []spec.Resource {
	if a.state == StatePresent {
		return []spec.Resource{spec.GroupResource(a.name)}
	}
	return nil
}

func (a *groupAction) Ops() []spec.Op {
	nameSource := a.step.Fields["name"].Value

	switch a.state {
	case StateAbsent:
		op := &removeGroupOp{
			name:       a.name,
			nameSource: nameSource,
		}
		op.SetAction(a)
		return []spec.Op{op}

	default:
		op := &ensureGroupOp{
			name:       a.name,
			gid:        a.gid,
			system:     a.system,
			nameSource: nameSource,
		}
		op.SetAction(a)
		return []spec.Op{op}
	}
}
