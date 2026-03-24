// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const ensurePkgID = "builtin.ensure-pkg"

// cacheStaleThreshold is the maximum age we consider a package cache "fresh".
// If CacheAge reports a younger cache, we skip the refresh on install failure.
// Set low (1s) so backends that report real ages still effectively always
// refresh — raise once we trust the heuristic.
const cacheStaleThreshold = 1 * time.Second

type ensurePkgOp struct {
	sharedops.BaseOp
	packages   []string
	state      State
	pkgsSource spec.SourceSpan
}

func (op *ensurePkgOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	pm := target.Must[target.PkgManager](ensurePkgID, tgt)

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
		}
	}

	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

func (op *ensurePkgOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	pm := target.Must[target.PkgManager](ensurePkgID, tgt)

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
		}
	}

	if len(actionable) == 0 {
		return spec.Result{Changed: false}, nil
	}

	switch op.state {
	case StatePresent:
		if installErr := pm.InstallPkgs(ctx, actionable); installErr != nil {
			retried, retryErr := op.retryInstallWithCacheRefresh(ctx, tgt, actionable)
			if retryErr != nil {
				return spec.Result{}, retryErr
			}
			if !retried {
				return spec.Result{}, PkgInstallError{
					Pkgs:   actionable,
					Stderr: installErr.Error(),
					Source: op.pkgsSource,
				}
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

// retryInstallWithCacheRefresh attempts to refresh the package cache and
// retry the install. Returns (true, nil) on successful retry, (true, err)
// with a proper diagnostic on retry failure, or (false, nil) when retry
// was not attempted (no PkgUpdater, or cache is fresh).
func (op *ensurePkgOp) retryInstallWithCacheRefresh(
	ctx context.Context,
	tgt target.Target,
	pkgs []string,
) (bool, error) {
	updater, ok := tgt.(target.PkgUpdater)
	if !ok {
		return false, nil
	}

	age, err := updater.CacheAge(ctx)
	if err != nil && !errors.Is(err, target.ErrNoCacheInfo) {
		return false, nil
	}
	if err == nil && age < cacheStaleThreshold {
		return false, nil
	}

	if err := updater.UpdateCache(ctx); err != nil {
		return true, PkgCacheError{
			Stderr: err.Error(),
			Source: op.pkgsSource,
		}
	}

	pm := target.Must[target.PkgManager](ensurePkgID, tgt)
	if err := pm.InstallPkgs(ctx, pkgs); err != nil {
		return true, PkgInstallError{
			Pkgs:   pkgs,
			Stderr: err.Error(),
			Source: op.pkgsSource,
		}
	}

	return true, nil
}

func (ensurePkgOp) RequiredCapabilities() capability.Capability {
	return capability.Pkg
}

func (op *ensurePkgOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "packages", Value: strings.Join(op.packages, ", ")},
		{Label: "state", Value: op.state.String()},
	}
}

// ensureLatestPkgOp
// -----------------------------------------------------------------------------

type ensureLatestPkgOp struct {
	sharedops.BaseOp
	packages   []string
	pkgsSource spec.SourceSpan
}

func (op *ensureLatestPkgOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	t := target.Must[interface {
		target.PkgManager
		target.PkgUpdater
	}](ensurePkgID, tgt)

	if err := t.UpdateCache(ctx); err != nil {
		return spec.CheckUnsatisfied, nil, PkgCacheError{
			Stderr: err.Error(),
			Source: op.pkgsSource,
		}
	}

	var drift []spec.DriftDetail
	for _, pkg := range op.packages {
		installed, err := t.IsInstalled(ctx, pkg)
		if err != nil {
			return spec.CheckUnsatisfied, nil, err
		}

		if !installed {
			drift = append(drift, spec.DriftDetail{
				Field:   "state",
				Current: pkg + ": not installed",
				Desired: "latest",
			})
		} else {
			upgradable, err := t.IsUpgradable(ctx, pkg)
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

	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

func (op *ensureLatestPkgOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	t := target.Must[interface {
		target.PkgManager
		target.PkgUpdater
	}](ensurePkgID, tgt)

	var actionable []string
	for _, pkg := range op.packages {
		installed, err := t.IsInstalled(ctx, pkg)
		if err != nil {
			return spec.Result{}, err
		}

		if !installed {
			actionable = append(actionable, pkg)
		} else {
			upgradable, err := t.IsUpgradable(ctx, pkg)
			if err != nil {
				return spec.Result{}, err
			}
			if upgradable {
				actionable = append(actionable, pkg)
			}
		}
	}

	if len(actionable) == 0 {
		return spec.Result{Changed: false}, nil
	}

	if err := t.InstallPkgs(ctx, actionable); err != nil {
		return spec.Result{}, PkgInstallError{
			Pkgs:   actionable,
			Stderr: err.Error(),
			Source: op.pkgsSource,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (ensureLatestPkgOp) RequiredCapabilities() capability.Capability {
	return capability.Pkg | capability.PkgUpdate
}

func (op *ensureLatestPkgOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "packages", Value: strings.Join(op.packages, ", ")},
	}
}

func (op *ensureLatestPkgOp) OpDescription() spec.OpDescription {
	return ensurePkgDesc{
		Pkgs:  fmt.Sprintf("[%s]", strings.Join(op.packages, ", ")),
		State: StateLatest,
	}
}

type ensurePkgDesc struct {
	Pkgs  string
	State State
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
