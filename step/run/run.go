// SPDX-License-Identifier: GPL-3.0-only

package run

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

type (
	Run       struct{}
	RunConfig struct {
		_ struct{} `summary:"Run an arbitrary shell command with optional check for idempotency"`

		Desc  string `step:"Human-readable description" optional:"true"`
		Apply string `step:"Shell command to execute" example:"sysctl -w net.ipv4.ip_forward=1"`
		Check string `step:"Exit 0 = already satisfied" optional:"true" exclusive:"trigger"`
		//nolint:revive // line-length unavoidable: long tag set; splitting tags hurts readability
		Always   bool              `step:"Always run apply, skip check" optional:"true" exclusive:"trigger" default:"false"`
		Env      map[string]string `step:"Environment variables for apply and check" optional:"true"`
		Promises []string          `step:"Resources this step produces (cross-deploy ordering)" optional:"true"`
		Inputs   []string          `step:"Resources this step requires (cross-deploy ordering)" optional:"true"`
	}
	runAction struct {
		desc   string
		apply  string
		check  string
		always bool
		env    map[string]string
		step   spec.StepInstance
	}
)

func (Run) Kind() string   { return "run" }
func (Run) NewConfig() any { return &RunConfig{} }

func (c *RunConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (Run) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*RunConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &RunConfig{}, step.Config)
	}

	if err := cfg.Validate(step); err != nil {
		return nil, err
	}

	return &runAction{
		desc:   cfg.Desc,
		apply:  cfg.Apply,
		check:  cfg.Check,
		always: cfg.Always,
		env:    cfg.Env,
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
		env:    a.env,
	}
	op.SetAction(a)
	return []spec.Op{op}
}
