// SPDX-License-Identifier: GPL-3.0-only

package run

import (
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

type (
	Run       struct{}
	RunConfig struct {
		Desc     string
		Apply    string
		Check    string
		Always   bool
		Env      map[string]string
		Provides []string
		Requires []string
	}
	runStep struct {
		desc   string
		apply  string
		check  string
		always bool
		env    map[string]string
		step   spec.DeclaredStep
	}
)

func (Run) Kind() string   { return "run" }
func (Run) NewConfig() any { return &RunConfig{} }

func (c *RunConfig) Resources() (provides, requires []string) {
	return c.Provides, c.Requires
}

func (Run) Plan(step spec.DeclaredStep) (spec.Step, error) {
	cfg, ok := step.Config.(*RunConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &RunConfig{}, step.Config)
	}

	if err := cfg.Validate(step); err != nil {
		return nil, err
	}

	return &runStep{
		desc:   cfg.Desc,
		apply:  cfg.Apply,
		check:  cfg.Check,
		always: cfg.Always,
		env:    cfg.Env,
		step:   step,
	}, nil
}

func (c *RunConfig) Validate(step spec.DeclaredStep) error {
	if c.Check != "" && c.Always {
		return CheckAlwaysConflictError{
			Span: step.Span,
		}
	}
	if c.Check == "" && !c.Always {
		return MissingCheckOrAlwaysError{
			Span: step.Span,
		}
	}
	return nil
}

func (a *runStep) Desc() string { return a.desc }
func (a *runStep) Kind() string { return "run" }

func (a *runStep) Ops() []spec.Op {
	op := &runOp{
		apply:  a.apply,
		check:  a.check,
		always: a.always,
		env:    a.env,
	}
	op.SetStep(a)
	return []spec.Op{op}
}
