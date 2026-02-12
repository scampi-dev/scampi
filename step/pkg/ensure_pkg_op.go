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
	packages   []string
	state      string
	pkgsSource spec.SourceSpan
}

func (op *ensurePkgOp) Check(
	ctx context.Context, _ source.Source, tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	pm := target.Must[target.PkgManager](ensurePkgID, tgt)

	if op.state == StateLatest {
		pu, ok := tgt.(target.PkgUpdater)
		if !ok {
			return spec.CheckUnsatisfied, nil, fmt.Errorf("target %T does not support upgrade checks", tgt)
		}
		if err := pu.UpdateCache(ctx); err != nil {
			return spec.CheckUnsatisfied, nil, PkgCacheError{
				Stderr: err.Error(),
				Source: op.pkgsSource,
			}
		}
	}

	var drift []spec.DriftDetail
	for _, pkg := range op.packages {
		installed, err := pm.IsInstalled(ctx, pkg)
		if err != nil {
			return spec.CheckUnsatisfied, nil, err
		}

		switch op.state {
		case StatePresent:
			if !installed {
				drift = append(drift, spec.DriftDetail{
					Field:   "state",
					Current: pkg + ": not installed",
					Desired: "present",
				})
			}
		case StateAbsent:
			if installed {
				drift = append(drift, spec.DriftDetail{
					Field:   "state",
					Current: pkg + ": installed",
					Desired: "absent",
				})
			}
		case StateLatest:
			if !installed {
				drift = append(drift, spec.DriftDetail{
					Field:   "state",
					Current: pkg + ": not installed",
					Desired: "latest",
				})
			} else {
				pu := tgt.(target.PkgUpdater) // safe: asserted above
				upgradable, err := pu.IsUpgradable(ctx, pkg)
				if err != nil {
					return spec.CheckUnsatisfied, nil, err
				}
				if upgradable {
					drift = append(drift, spec.DriftDetail{
						Field:   "state",
						Current: pkg + ": upgradable",
						Desired: "latest",
					})
				}
			}
		}
	}

	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
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

		switch op.state {
		case StatePresent:
			if !installed {
				actionable = append(actionable, pkg)
			}
		case StateAbsent:
			if installed {
				actionable = append(actionable, pkg)
			}
		case StateLatest:
			if !installed {
				actionable = append(actionable, pkg)
			} else {
				pu := tgt.(target.PkgUpdater) // safe: Check already asserted
				upgradable, err := pu.IsUpgradable(ctx, pkg)
				if err != nil {
					return spec.Result{}, err
				}
				if upgradable {
					actionable = append(actionable, pkg)
				}
			}
		}
	}

	if len(actionable) == 0 {
		return spec.Result{Changed: false}, nil
	}

	switch op.state {
	case StatePresent, StateLatest:
		if err := pm.InstallPkgs(ctx, actionable); err != nil {
			return spec.Result{}, PkgInstallError{
				Pkgs:   actionable,
				Stderr: err.Error(),
				Source: op.pkgsSource,
			}
		}
	case StateAbsent:
		if err := pm.RemovePkgs(ctx, actionable); err != nil {
			return spec.Result{}, PkgRemoveError{
				Pkgs:   actionable,
				Stderr: err.Error(),
				Source: op.pkgsSource,
			}
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
