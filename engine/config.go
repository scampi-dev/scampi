// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"path/filepath"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/star"
)

// LoadConfig decodes and validates user configuration.
// It returns ONLY user-facing configuration errors.
// All other failures are engine or environment bugs and will panic.
func LoadConfig(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
) (spec.Config, error) {
	cfgPath, absErr := filepath.Abs(cfgPath)
	if absErr != nil {
		panic(errs.BUG("filepath.Abs() failed: %w", absErr))
	}

	cfg, err := star.Eval(ctx, cfgPath, store, src)
	if err != nil {
		impact, _ := emitEngineDiagnostic(em, cfgPath, err)
		if impact.ShouldAbort() {
			return spec.Config{}, AbortError{Causes: []error{err}}
		}
		return spec.Config{}, err
	}

	return cfg, nil
}

// ResolveMultiple produces ResolvedConfigs for all matching (deploy, target)
// combinations based on the provided options.
func ResolveMultiple(cfg spec.Config, opts spec.ResolveOptions) ([]spec.ResolvedConfig, error) {
	var deployNames []string
	if len(opts.DeployNames) > 0 {
		for _, name := range opts.DeployNames {
			if _, ok := cfg.Deploy[name]; !ok {
				return nil, UnknownDeployBlockError{Name: name}
			}
		}
		deployNames = opts.DeployNames
	} else {
		for name := range cfg.Deploy {
			deployNames = append(deployNames, name)
		}
	}

	if len(deployNames) == 0 {
		return nil, NoDeployBlocksError{}
	}

	var results []spec.ResolvedConfig

	for _, deployName := range deployNames {
		block := cfg.Deploy[deployName]

		var targetNames []string
		if len(opts.TargetNames) > 0 {
			blockTargets := make(map[string]bool)
			for _, t := range block.Targets {
				blockTargets[t] = true
			}
			for _, t := range opts.TargetNames {
				if blockTargets[t] {
					targetNames = append(targetNames, t)
				}
			}
			if len(targetNames) == 0 {
				continue
			}
		} else {
			targetNames = block.Targets
		}

		if len(targetNames) == 0 {
			return nil, NoTargetsInDeployError{Deploy: deployName}
		}

		for _, targetName := range targetNames {
			tgt, ok := cfg.Targets[targetName]
			if !ok {
				return nil, UnknownTargetError{
					Name:   targetName,
					Deploy: deployName,
				}
			}

			results = append(results, spec.ResolvedConfig{
				Path:       cfg.Path,
				DeployName: deployName,
				TargetName: targetName,
				Target:     tgt,
				Steps:      block.Steps,
			})
		}
	}

	if len(results) == 0 {
		return nil, NoDeployBlocksError{}
	}

	return results, nil
}

// Resolve produces a ResolvedConfig from a Config by selecting a specific
// deploy block and target. If deployName or targetName are empty, the first
// available is selected.
func Resolve(cfg spec.Config, deployName, targetName string) (spec.ResolvedConfig, error) {
	var block spec.DeployBlock
	if deployName != "" {
		var ok bool
		block, ok = cfg.Deploy[deployName]
		if !ok {
			return spec.ResolvedConfig{}, UnknownDeployBlockError{Name: deployName}
		}
	} else {
		// Pick first deploy block (map iteration order is random, but for
		// single-block configs this is fine)
		for name, b := range cfg.Deploy {
			block = b
			deployName = name
			break
		}
		if deployName == "" {
			return spec.ResolvedConfig{}, NoDeployBlocksError{}
		}
	}

	if targetName == "" {
		if len(block.Targets) == 0 {
			return spec.ResolvedConfig{}, NoTargetsInDeployError{Deploy: deployName}
		}
		targetName = block.Targets[0]
	}

	tgt, ok := cfg.Targets[targetName]
	if !ok {
		return spec.ResolvedConfig{}, UnknownTargetError{
			Name:   targetName,
			Deploy: deployName,
		}
	}

	var found bool
	for _, t := range block.Targets {
		if t == targetName {
			found = true
			break
		}
	}
	if !found {
		return spec.ResolvedConfig{}, TargetNotInDeployError{
			Target: targetName,
			Deploy: deployName,
		}
	}

	return spec.ResolvedConfig{
		Path:       cfg.Path,
		DeployName: deployName,
		TargetName: targetName,
		Target:     tgt,
		Steps:      block.Steps,
	}, nil
}
