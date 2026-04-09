// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"path/filepath"
	"slices"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/linker"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
)

// ConfigLoader loads and evaluates a configuration file, returning
// a spec.Config. Different frontends (Starlark, scampi-lang) provide
// different loaders.
type ConfigLoader func(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	src source.Source,
) (spec.Config, error)

// LoadConfig decodes and validates user configuration by running the
// scampi-lang pipeline (lex → parse → check → eval → link).
func LoadConfig(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	src source.Source,
) (spec.Config, error) {
	cfgPath, absErr := filepath.Abs(cfgPath)
	if absErr != nil {
		panic(errs.BUG("filepath.Abs() failed: %w", absErr))
	}

	// Add source file to store for diagnostic source rendering.
	if store != nil {
		if data, readErr := src.ReadFile(ctx, cfgPath); readErr == nil {
			store.AddFile(cfgPath, data)
		}
	}

	reg := NewRegistry()
	cfg, err := linker.LoadConfig(ctx, cfgPath, src, reg)
	if err != nil {
		impact, emitted := emitEngineDiagnostic(em, cfgPath, err)
		if emitted && impact.ShouldAbort() {
			return spec.Config{}, AbortError{Causes: []error{err}}
		}
		// Non-diagnostic errors (parse, check, eval) — treat as abort.
		return spec.Config{}, AbortError{Causes: []error{err}}
	}

	return cfg, nil
}

// ResolveMultiple produces ResolvedConfigs for all matching (deploy, target)
// combinations based on the provided options.
func ResolveMultiple(cfg spec.Config, opts spec.ResolveOptions) ([]spec.ResolvedConfig, error) {
	var blocks []spec.DeployBlock
	if len(opts.DeployNames) > 0 {
		for _, name := range opts.DeployNames {
			b, ok := cfg.DeployByName(name)
			if !ok {
				return nil, UnknownDeployBlockError{Name: name}
			}
			blocks = append(blocks, b)
		}
	} else {
		blocks = cfg.Deploy
	}

	if len(blocks) == 0 {
		return nil, NoDeployBlocksError{}
	}

	var results []spec.ResolvedConfig

	for _, block := range blocks {
		deployName := block.Name

		var targetNames []string
		if len(opts.TargetNames) > 0 {
			for _, t := range opts.TargetNames {
				if slices.Contains(block.Targets, t) {
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
				Hooks:      block.Hooks,
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
		block, ok = cfg.DeployByName(deployName)
		if !ok {
			return spec.ResolvedConfig{}, UnknownDeployBlockError{Name: deployName}
		}
	} else {
		if len(cfg.Deploy) == 0 {
			return spec.ResolvedConfig{}, NoDeployBlocksError{}
		}
		block = cfg.Deploy[0]
		deployName = block.Name
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

	if !slices.Contains(block.Targets, targetName) {
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
		Hooks:      block.Hooks,
	}, nil
}
