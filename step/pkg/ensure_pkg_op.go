package pkg

import (
	"context"
	"fmt"
	"strings"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/target"
)

const ensurePkgID = "builtin.ensure-pkg"

type ensurePkgOp struct {
	sharedops.BaseOp
	packages []string
	state    string
}

func (op *ensurePkgOp) Check(ctx context.Context, _ source.Source, tgt target.Target) (spec.CheckResult, error) {
	pm := target.Must[target.PkgManager](ensurePkgID, tgt)

	for _, pkg := range op.packages {
		installed, err := pm.IsInstalled(ctx, pkg)
		if err != nil {
			return spec.CheckUnsatisfied, err
		}

		switch op.state {
		case StatePresent:
			if !installed {
				return spec.CheckUnsatisfied, nil
			}
		case StateAbsent:
			if installed {
				return spec.CheckUnsatisfied, nil
			}
		}
	}

	return spec.CheckSatisfied, nil
}

func (op *ensurePkgOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	pm := target.Must[target.PkgManager](ensurePkgID, tgt)

	// Collect only the packages that need action.
	var actionable []string
	for _, pkg := range op.packages {
		installed, err := pm.IsInstalled(ctx, pkg)
		if err != nil {
			return spec.Result{}, err
		}

		needsAction := (op.state == StatePresent && !installed) ||
			(op.state == StateAbsent && installed)
		if needsAction {
			actionable = append(actionable, pkg)
		}
	}

	if len(actionable) == 0 {
		return spec.Result{Changed: false}, nil
	}

	switch op.state {
	case StatePresent:
		if err := pm.InstallPkgs(ctx, actionable); err != nil {
			return spec.Result{}, err
		}
	case StateAbsent:
		if err := pm.RemovePkgs(ctx, actionable); err != nil {
			return spec.Result{}, err
		}
	}

	return spec.Result{Changed: true}, nil
}

func (ensurePkgOp) RequiredCapabilities() capability.Capability {
	return capability.Pkg
}

type ensurePkgDesc struct {
	Pkgs  string
	State string
}

func (d ensurePkgDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensurePkgID,
		Text: `ensure pkgs {{.Pkgs}} are {{.State}}`,
		Data: d,
	}
}

func (op *ensurePkgOp) OpDescription() spec.OpDescription {
	return ensurePkgDesc{
		Pkgs:  fmt.Sprintf("[%s]", strings.Join(op.packages, ", ")),
		State: op.state,
	}
}
