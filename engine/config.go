// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"path/filepath"
	"slices"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/linker"
	"scampi.dev/scampi/mod"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
)

// LoadConfig decodes and validates user configuration by running the
// scampi pipeline (lex → parse → check → eval → link).
func LoadConfig(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	src source.Source,
	opts ...linker.AnalyzeOption,
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

	// Auto-attach the redactor from ctx as a linker option so secret
	// values flow into render-time redaction without callers having
	// to pass it through every engine entry point.
	if r := secret.FromContext(ctx); r != nil {
		opts = append(opts, linker.WithRedactor(r))
	}

	// Apply-time scampi.sum pre-flight (#310). If the project has a
	// scampi.mod, verify every required module's cache dir against
	// the hash recorded in scampi.sum *before* the linker reads any
	// module source. A mismatch means the cache was tampered or a
	// previous fetch never completed — either way, refuse to run.
	if err := verifyModuleSums(ctx, cfgPath, src, em); err != nil {
		return spec.Config{}, AbortError{Causes: []error{err}}
	}

	reg := NewRegistry()
	cfg, err := linker.LoadConfig(ctx, cfgPath, src, reg, opts...)
	if err != nil {
		_, emitted := emitEngineDiagnostic(em, cfgPath, err)
		if !emitted {
			// Error didn't carry a diagnostic (e.g. raw file-read
			// failure). Wrap it in a generic LoadConfigError so the
			// user sees something instead of a silent abort.
			em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic(
				cfgPath,
				&LoadConfigError{
					Cause:  err,
					Source: spec.SourceSpan{Filename: cfgPath},
				},
			))
		}
		return spec.Config{}, AbortError{Causes: []error{err}}
	}

	return cfg, nil
}

// verifyModuleSums walks up from cfgPath to find scampi.mod, parses it,
// and runs mod.VerifyAll against the default cache. Returns nil if
// there's no scampi.mod (raw .scampi without modules), or if every
// require entry's cache matches scampi.sum. Mismatches are routed
// through the diagnostic emitter so the user sees the same message
// `scampi mod verify` would produce.
func verifyModuleSums(
	ctx context.Context,
	cfgPath string,
	src source.Source,
	em diagnostic.Emitter,
) error {
	modFile := findModFileUp(ctx, src, cfgPath)
	if modFile == "" {
		return nil
	}
	data, err := src.ReadFile(ctx, modFile)
	if err != nil {
		return nil // no scampi.mod readable here; treat as no-module project
	}
	m, err := mod.Parse(modFile, data)
	if err != nil {
		return nil // parse errors surface via the lang pipeline; not our concern
	}
	if err := mod.VerifyAll(ctx, src, m, mod.DefaultCacheDir()); err != nil {
		_, emitted := emitEngineDiagnostic(em, cfgPath, err)
		if !emitted {
			em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic(
				cfgPath,
				&LoadConfigError{Cause: err, Source: spec.SourceSpan{Filename: cfgPath}},
			))
		}
		return err
	}
	return nil
}

// findModFileUp walks up from cfgPath looking for a scampi.mod file
// via the source interface (engine forbids direct os imports).
// Mirrors linker.findModFile (unexported there) — duplicated to keep
// the engine→linker dep one-way.
func findModFileUp(ctx context.Context, src source.Source, cfgPath string) string {
	dir := filepath.Dir(cfgPath)
	for {
		candidate := filepath.Join(dir, "scampi.mod")
		if meta, err := src.Stat(ctx, candidate); err == nil && meta.Exists {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// ResolveMultiple produces ResolvedConfigs for all matching (deploy, target)
// combinations based on the provided options.
func ResolveMultiple(cfg spec.Config, opts spec.ResolveOptions) ([]spec.ResolvedConfig, error) {
	cfgSpan := spec.SourceSpan{Filename: cfg.Path}

	var blocks []spec.DeployBlock
	if len(opts.DeployNames) > 0 {
		for _, name := range opts.DeployNames {
			b, ok := cfg.DeployByName(name)
			if !ok {
				return nil, UnknownDeployBlockError{Name: name, Source: cfgSpan}
			}
			blocks = append(blocks, b)
		}
	} else {
		blocks = cfg.Deploy
	}

	if len(blocks) == 0 {
		return nil, NoDeployBlocksError{Source: cfgSpan}
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
			return nil, NoTargetsInDeployError{Deploy: deployName, Source: block.Source}
		}

		for _, targetName := range targetNames {
			tgt, ok := cfg.Targets[targetName]
			if !ok {
				return nil, UnknownTargetError{
					Name:   targetName,
					Deploy: deployName,
					Source: block.Source,
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
			return spec.ResolvedConfig{}, NoTargetsInDeployError{
				Deploy: deployName,
				Source: block.Source,
			}
		}
		targetName = block.Targets[0]
	}

	tgt, ok := cfg.Targets[targetName]
	if !ok {
		return spec.ResolvedConfig{}, UnknownTargetError{
			Name:   targetName,
			Deploy: deployName,
			Source: block.Source,
		}
	}

	if !slices.Contains(block.Targets, targetName) {
		return spec.ResolvedConfig{}, TargetNotInDeployError{
			Target: targetName,
			Deploy: deployName,
			Source: block.Source,
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
