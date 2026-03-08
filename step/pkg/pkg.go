// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

// Desired package state values.
const (
	StatePresent = "present"
	StateAbsent  = "absent"
	StateLatest  = "latest"
)

type (
	Pkg       struct{}
	PkgConfig struct {
		_ struct{} `summary:"Ensure packages are present, absent, or at the latest version on the target"`

		Desc     string   `step:"Human-readable description" optional:"true"`
		Packages []string `step:"Packages to manage" example:"[\"nginx\", \"curl\"]"`
		State    string   `step:"Desired package state" default:"present" example:"latest"`
	}
	pkgAction struct {
		idx      int
		desc     string
		packages []string
		state    string
		step     spec.StepInstance
	}
)

func (Pkg) Kind() string   { return "pkg" }
func (Pkg) NewConfig() any { return &PkgConfig{} }

func (c *PkgConfig) Validate(step spec.StepInstance) error {
	if len(c.Packages) == 0 {
		return EmptyPackagesError{
			Source: step.Fields["packages"].Value,
		}
	}
	switch c.State {
	case StatePresent, StateAbsent, StateLatest:
	default:
		return InvalidStateError{
			Got:     c.State,
			Allowed: []string{StatePresent, StateAbsent, StateLatest},
			Source:  step.Fields["state"].Value,
		}
	}
	return nil
}

func (p Pkg) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*PkgConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &PkgConfig{}, step.Config)
	}

	if err := cfg.Validate(step); err != nil {
		return nil, err
	}

	return &pkgAction{
		idx:      idx,
		desc:     cfg.Desc,
		packages: cfg.Packages,
		state:    cfg.State,
		step:     step,
	}, nil
}

func (a *pkgAction) Desc() string { return a.desc }
func (a *pkgAction) Kind() string { return "pkg" }

func (a *pkgAction) Ops() []spec.Op {
	pkgsSource := a.step.Fields["packages"].Value

	// Two branches because SetAction lives on the concrete BaseOp, not on
	// the spec.Op interface, so we need the concrete type to wire it up.
	var op spec.Op
	if a.state == StateLatest {
		o := &ensureLatestPkgOp{packages: a.packages, pkgsSource: pkgsSource}
		o.SetAction(a)
		op = o
	} else {
		o := &ensurePkgOp{packages: a.packages, state: a.state, pkgsSource: pkgsSource}
		o.SetAction(a)
		op = o
	}
	return []spec.Op{op}
}
