// Package pkg implements the pkg step type for declarative package management.
package pkg

import (
	"godoit.dev/doit/errs"
	"godoit.dev/doit/spec"
)

// Desired package state values.
const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

type (
	Pkg       struct{}
	PkgConfig struct {
		Desc     string
		Packages []string
		State    string
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

func (p Pkg) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*PkgConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &PkgConfig{}, step.Config)
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
	op := &ensurePkgOp{
		packages: a.packages,
		state:    a.state,
	}
	op.SetAction(a)
	return []spec.Op{op}
}
