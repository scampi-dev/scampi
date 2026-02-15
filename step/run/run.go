// SPDX-License-Identifier: GPL-3.0-only

// Package run implements the run step type — doit's escape hatch for
// arbitrary shell commands that preserve the check/execute contract.
package run

import (
	"godoit.dev/doit/errs"
	"godoit.dev/doit/spec"
)

type (
	Run       struct{}
	RunConfig struct {
		_ struct{} `summary:"Run an arbitrary shell command with optional check for idempotency"`

		Desc   string `step:"Human-readable description" optional:"true"`
		Apply  string `step:"Shell command to execute" example:"sysctl -w net.ipv4.ip_forward=1"`
		Check  string `step:"Exit 0 = already satisfied" optional:"true" example:"grep -q ok /tmp/status"`
		Always bool   `step:"Always run apply, skip check" optional:"true" default:"false"`
	}
	runAction struct {
		idx    int
		desc   string
		apply  string
		check  string
		always bool
		step   spec.StepInstance
	}
)

func (Run) Kind() string   { return "run" }
func (Run) NewConfig() any { return &RunConfig{} }

func (Run) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*RunConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &RunConfig{}, step.Config)
	}

	if err := cfg.Validate(step); err != nil {
		return nil, err
	}

	return &runAction{
		idx:    idx,
		desc:   cfg.Desc,
		apply:  cfg.Apply,
		check:  cfg.Check,
		always: cfg.Always,
		step:   step,
	}, nil
}

func (c *RunConfig) Validate(step spec.StepInstance) error {
	if c.Check != "" && c.Always {
		return CheckAlwaysConflictError{
			Source: step.Source,
		}
	}
	if c.Check == "" && !c.Always {
		return MissingCheckOrAlwaysError{
			Source: step.Source,
		}
	}
	return nil
}

func (a *runAction) Desc() string { return a.desc }
func (a *runAction) Kind() string { return "run" }

func (a *runAction) Ops() []spec.Op {
	op := &runOp{
		apply:  a.apply,
		check:  a.check,
		always: a.always,
	}
	op.SetAction(a)
	return []spec.Op{op}
}
