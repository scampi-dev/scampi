// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"context"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const writeRepoConfigID = "step.write-repo-config"

type writeRepoConfigOp struct {
	sharedops.BaseOp
	source spec.PkgSourceRef
}

func (op *writeRepoConfigOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	rm := target.Must[target.RepoManager](writeRepoConfigID, tgt)

	has, err := rm.HasRepo(ctx, op.source.Name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if has {
		return spec.CheckSatisfied, nil, nil
	}
	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "repo_config",
		Current: "absent",
		Desired: "present",
	}}, nil
}

func (op *writeRepoConfigOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	rm := target.Must[target.RepoManager](writeRepoConfigID, tgt)

	suite := op.source.Suite
	if suite == "" && op.source.Kind == spec.PkgSourceApt {
		oip, ok := tgt.(target.OSInfoProvider)
		if !ok || oip.VersionCodename() == "" {
			return spec.Result{}, SuiteDetectionError{
				Name: op.source.Name,
			}
		}
		suite = oip.VersionCodename()
	}

	components := op.source.Components
	if len(components) == 0 && op.source.Kind == spec.PkgSourceApt {
		// Default to "main" when components not specified for apt
		components = []string{"main"}
	}

	keyPath := rm.RepoKeyPath(op.source.Name)
	cfg := target.RepoConfig{
		Name:       op.source.Name,
		URL:        op.source.URL,
		KeyPath:    keyPath,
		ConfigPath: rm.RepoConfigPath(op.source.Name),
		Suite:      suite,
		Components: components,
	}

	if err := rm.WriteRepoConfig(ctx, cfg); err != nil {
		return spec.Result{}, RepoConfigError{
			Name:   op.source.Name,
			Detail: err.Error(),
		}
	}

	// Refresh package cache after writing repo config.
	if updater, ok := tgt.(target.PkgUpdater); ok {
		if err := updater.UpdateCache(ctx); err != nil {
			return spec.Result{}, PkgCacheError{
				Stderr: err.Error(),
			}
		}
	}

	return spec.Result{Changed: true}, nil
}

func (writeRepoConfigOp) RequiredCapabilities() capability.Capability {
	return capability.PkgRepo
}

type writeRepoConfigDesc struct {
	Name string
}

func (d writeRepoConfigDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   writeRepoConfigID,
		Text: `configure repo source "{{.Name}}"`,
		Data: d,
	}
}

func (op *writeRepoConfigOp) OpDescription() spec.OpDescription {
	return writeRepoConfigDesc{Name: op.source.Name}
}
