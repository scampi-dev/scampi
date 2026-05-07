// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/mod"
	"scampi.dev/scampi/source"
)

// scampi mod
// -----------------------------------------------------------------------------

func modCmd() *cli.Command {
	return &cli.Command{
		Name:                   "mod",
		Usage:                  "Manage scampi module dependencies",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Commands: []*cli.Command{
			modInitCmd(),
			modTidyCmd(),
			modAddCmd(),
			modDownloadCmd(),
			modUpdateCmd(),
			modVerifyCmd(),
			modCacheCmd(),
			modCleanCmd(),
		},
	}
}

// scampi mod init
// -----------------------------------------------------------------------------

func modInitCmd() *cli.Command {
	var modulePath string

	return &cli.Command{
		Name:         "init",
		Usage:        "Create a scampi.mod file in the current directory",
		ArgsUsage:    "[module-path]",
		OnUsageError: onUsageError,
		Before:       requireMaxArgs(1),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "module-path",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &modulePath,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()

			pol := cliPolicy(opts)
			em := diagnostic.NewEmitter(pol, displ)

			dir, err := os.Getwd()
			if err != nil {
				panic(errs.BUG("os.Getwd failed: %w", err))
			}
			src := source.LocalPosixSource{}
			if err := mod.Init(ctx, src, dir, modulePath); err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod init", engine.AbortError{Causes: []error{err}})
			}
			emitModInfo(em, "created scampi.mod")
			return nil
		},
	}
}

// scampi mod tidy
// -----------------------------------------------------------------------------

func modTidyCmd() *cli.Command {
	return &cli.Command{
		Name:         "tidy",
		Usage:        "Sync the require block with load() calls in *.scampi files",
		OnUsageError: onUsageError,
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()

			pol := cliPolicy(opts)
			em := diagnostic.NewEmitter(pol, displ)

			dir, err := os.Getwd()
			if err != nil {
				panic(errs.BUG("os.Getwd failed: %w", err))
			}
			src := source.LocalPosixSource{}
			changes, err := mod.Tidy(ctx, src, dir)
			if err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod tidy", engine.AbortError{Causes: []error{err}})
			}
			if len(changes) == 0 {
				emitModInfo(em, "scampi.mod is up to date")
				return nil
			}
			for _, c := range changes {
				emitModInfo(em, c)
			}
			return nil
		},
	}
}

// scampi mod add
// -----------------------------------------------------------------------------

func modAddCmd() *cli.Command {
	var moduleArg, pathArg string

	return &cli.Command{
		Name:  "add",
		Usage: "Add a dependency to scampi.mod",
		ArgsUsage: `<module[@version]>
   scampi mod add <name> <local-path>`,
		OnUsageError: onUsageError,
		Before:       requireArgsRange(1, 2),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "module",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &moduleArg,
			},
			&cli.StringArg{
				Name:        "path",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &pathArg,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()

			pol := cliPolicy(opts)
			em := diagnostic.NewEmitter(pol, displ)

			dir, err := os.Getwd()
			if err != nil {
				panic(errs.BUG("os.Getwd failed: %w", err))
			}

			var modPath, version string
			if pathArg != "" {
				// Two-arg form: scampi mod add <name> <local-path>
				modPath = moduleArg
				version = pathArg
			} else {
				// One-arg form: scampi mod add <module[@version]>
				modPath, version = parseModArg(moduleArg)
			}
			cacheDir := mod.DefaultCacheDir()

			src := source.LocalPosixSource{}
			resolved, change, err := mod.Add(ctx, src, modPath, version, dir, cacheDir)
			if err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod add", engine.AbortError{Causes: []error{err}})
			}
			switch change {
			case mod.ModFileAdded:
				emitModInfo(em, fmt.Sprintf("added %s@%s", modPath, resolved))
			case mod.ModFileUpdated:
				emitModInfo(em, fmt.Sprintf("updated %s to %s", modPath, resolved))
			case mod.ModFileUnchanged:
				emitModInfo(em, fmt.Sprintf("%s@%s already in scampi.mod", modPath, resolved))
			}
			return nil
		},
	}
}

// scampi mod download
// -----------------------------------------------------------------------------

func modDownloadCmd() *cli.Command {
	return &cli.Command{
		Name:         "download",
		Usage:        "Download all dependencies listed in scampi.mod",
		OnUsageError: onUsageError,
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()

			pol := cliPolicy(opts)
			em := diagnostic.NewEmitter(pol, displ)

			dir, err := os.Getwd()
			if err != nil {
				panic(errs.BUG("os.Getwd failed: %w", err))
			}

			src := source.LocalPosixSource{}

			modFile := filepath.Join(dir, "scampi.mod")
			data, err := os.ReadFile(modFile)
			if err != nil {
				e := &mod.TidyError{Detail: "could not read scampi.mod: " + err.Error(), Hint: "run: scampi mod init"}
				emitModDiagnostic(em, e)
				return handleEngineError("mod download", engine.AbortError{Causes: []error{e}})
			}

			m, err := mod.Parse(modFile, data)
			if err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
			}

			sumFile := filepath.Join(dir, "scampi.sum")
			sums, err := mod.ReadSum(ctx, src, sumFile)
			if err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
			}

			cacheDir := mod.DefaultCacheDir()
			updated := false

			for _, dep := range m.Require {
				if dep.IsLocal() {
					continue
				}
				if err := mod.Fetch(m, dep, cacheDir); err != nil {
					emitModDiagnostic(em, err)
					return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
				}

				dest := filepath.Join(cacheDir, dep.Path+"@"+dep.Version)
				if err := mod.ValidateEntryPoint(ctx, source.LocalPosixSource{}, m, dep, dest); err != nil {
					emitModDiagnostic(em, err)
					return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
				}

				hash, err := mod.ComputeHash(dest)
				if err != nil {
					emitModDiagnostic(em, err)
					return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
				}

				key := dep.Path + " " + dep.Version
				if sums[key] == "" {
					sums[key] = hash
					updated = true
				}

				emitModInfo(em, fmt.Sprintf("downloaded %s@%s", dep.Path, dep.Version))
			}

			// Resolve and fetch transitive dependencies.
			allDeps, err := mod.FetchTransitive(ctx, src, m, cacheDir)
			if err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
			}

			for _, dep := range allDeps {
				if !dep.Indirect || dep.IsLocal() {
					continue
				}
				dest := filepath.Join(cacheDir, dep.Path+"@"+dep.Version)
				if err := mod.ValidateEntryPoint(ctx, source.LocalPosixSource{}, m, dep, dest); err != nil {
					emitModDiagnostic(em, err)
					return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
				}
				hash, err := mod.ComputeHash(dest)
				if err != nil {
					emitModDiagnostic(em, err)
					return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
				}
				key := dep.Path + " " + dep.Version
				if sums[key] == "" {
					sums[key] = hash
					updated = true
				}
				emitModInfo(em, fmt.Sprintf("downloaded %s@%s (indirect)", dep.Path, dep.Version))
			}

			if updated {
				if err := mod.WriteSum(ctx, src, sumFile, sums); err != nil {
					emitModDiagnostic(em, err)
					return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
				}
			}

			// Write scampi.mod with indirect markers.
			if err := mod.WriteModFile(ctx, src, modFile, m.Module, allDeps); err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod download", engine.AbortError{Causes: []error{err}})
			}

			if len(m.Require) == 0 && len(allDeps) == 0 {
				emitModInfo(em, "all modules up to date")
			}

			return nil
		},
	}
}

// scampi mod update
// -----------------------------------------------------------------------------

func modUpdateCmd() *cli.Command {
	var moduleArg string

	return &cli.Command{
		Name:         "update",
		Usage:        "Update a dependency to its latest stable version",
		ArgsUsage:    "<module>",
		OnUsageError: onUsageError,
		Before:       requireArgs(1),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "module",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &moduleArg,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()

			pol := cliPolicy(opts)
			em := diagnostic.NewEmitter(pol, displ)

			dir, err := os.Getwd()
			if err != nil {
				panic(errs.BUG("os.Getwd failed: %w", err))
			}

			src := source.LocalPosixSource{}
			modPath, version := parseModArg(moduleArg)
			cacheDir := mod.DefaultCacheDir()

			resolved, _, err := mod.Add(ctx, src, modPath, version, dir, cacheDir)
			if err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod update", engine.AbortError{Causes: []error{err}})
			}
			emitModInfo(em, fmt.Sprintf("updated %s to %s", modPath, resolved))
			return nil
		},
	}
}

// scampi mod verify
// -----------------------------------------------------------------------------

func modVerifyCmd() *cli.Command {
	return &cli.Command{
		Name:         "verify",
		Usage:        "Verify that cached modules match their checksums in scampi.sum",
		OnUsageError: onUsageError,
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()

			pol := cliPolicy(opts)
			em := diagnostic.NewEmitter(pol, displ)

			dir, err := os.Getwd()
			if err != nil {
				panic(errs.BUG("os.Getwd failed: %w", err))
			}

			src := source.LocalPosixSource{}

			modFile := filepath.Join(dir, "scampi.mod")
			data, err := os.ReadFile(modFile)
			if err != nil {
				e := &mod.TidyError{Detail: "could not read scampi.mod: " + err.Error(), Hint: "run: scampi mod init"}
				emitModDiagnostic(em, e)
				return handleEngineError("mod verify", engine.AbortError{Causes: []error{e}})
			}

			m, err := mod.Parse(modFile, data)
			if err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod verify", engine.AbortError{Causes: []error{err}})
			}

			sumFile := filepath.Join(dir, "scampi.sum")
			sums, err := mod.ReadSum(ctx, src, sumFile)
			if err != nil {
				emitModDiagnostic(em, err)
				return handleEngineError("mod verify", engine.AbortError{Causes: []error{err}})
			}

			cacheDir := mod.DefaultCacheDir()

			for _, dep := range m.Require {
				if dep.IsLocal() {
					continue
				}
				modDir := filepath.Join(cacheDir, dep.Path+"@"+dep.Version)
				if err := mod.ValidateEntryPoint(ctx, source.LocalPosixSource{}, m, dep, modDir); err != nil {
					emitModDiagnostic(em, err)
					return handleEngineError("mod verify", engine.AbortError{Causes: []error{err}})
				}
				if err := mod.VerifyModule(m, dep, modDir, sums); err != nil {
					emitModDiagnostic(em, err)
					return handleEngineError("mod verify", engine.AbortError{Causes: []error{err}})
				}
			}

			emitModInfo(em, "all modules verified")
			return nil
		},
	}
}

// scampi mod cache
// -----------------------------------------------------------------------------

func modCacheCmd() *cli.Command {
	return &cli.Command{
		Name:         "cache",
		Usage:        "Print the module cache directory",
		OnUsageError: onUsageError,
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()

			pol := cliPolicy(opts)
			em := diagnostic.NewEmitter(pol, displ)

			emitModInfo(em, mod.DefaultCacheDir())
			return nil
		},
	}
}

// scampi mod clean
// -----------------------------------------------------------------------------

func modCleanCmd() *cli.Command {
	return &cli.Command{
		Name:         "clean",
		Usage:        "Remove all cached modules",
		OnUsageError: onUsageError,
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()

			pol := cliPolicy(opts)
			em := diagnostic.NewEmitter(pol, displ)

			cacheDir := mod.DefaultCacheDir()
			if err := os.RemoveAll(cacheDir); err != nil {
				e := &mod.TidyError{
					Detail: "could not remove cache directory: " + err.Error(),
					Hint:   "check that " + cacheDir + " is accessible",
				}
				emitModDiagnostic(em, e)
				return handleEngineError("mod clean", engine.AbortError{Causes: []error{e}})
			}
			emitModInfo(em, fmt.Sprintf("cache cleared: %s", cacheDir))
			return nil
		},
	}
}

// Helpers
// -----------------------------------------------------------------------------

func emitModDiagnostic(em diagnostic.Emitter, err error) {
	if d, ok := err.(diagnostic.Diagnostic); ok {
		em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic("", d))
	}
}

func emitModInfo(em diagnostic.Emitter, detail string) {
	em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic("", &mod.ModInfo{
		Detail: detail,
	}))
}

func parseModArg(s string) (string, string) {
	if i := strings.LastIndex(s, "@"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}
