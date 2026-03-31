// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/osutil"
	"scampi.dev/scampi/spec"
)

// scampi inspect
// -----------------------------------------------------------------------------

func inspectCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:      "inspect",
		Usage:     "Show resolved state for all steps, or diff file content",
		ArgsUsage: "<config> [path]",
		Description: `Reads a declarative configuration file and shows the resolved state
of all steps after Starlark evaluation.

Use --diff to compare file content against the current target state:
  scampi inspect config.scampi --diff              # list diffable paths
  scampi inspect config.scampi --diff nginx.conf   # diff that file
  scampi inspect config.scampi --diff -i           # pick interactively

Set SCAMPI_DIFFTOOL, DIFFTOOL, or EDITOR to choose your diff tool.
Set SCAMPI_FUZZY_FINDER (e.g. fzf, sk) for interactive picking.
Falls back to plain diff(1).`,
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Before:                 requireArgsRange(1, 2),
		Flags: append(resolveFlags(),
			&cli.BoolFlag{
				Name:  "diff",
				Usage: "diff file content (add a path argument to select which file)",
			},
			&cli.BoolFlag{
				Name:    "interactive",
				Aliases: []string{"i"},
				Usage:   "pick a file interactively using $SCAMPI_FUZZY_FINDER (e.g. fzf)",
			},
		),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			isDiff := cmd.Bool("diff")
			interactive := cmd.Bool("interactive")
			diffPath := cmd.Args().Get(0)

			if interactive && !isDiff {
				return cli.Exit("--interactive requires --diff", exitUserError)
			}

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
				SuppressPlan:     isDiff && diffPath == "" && !interactive,
			}

			store := diagnostic.NewSourceStore()

			displ, cleanup := withDisplayer(opts, store)
			defer cleanup()

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)

			if isDiff && interactive && diffPath == "" {
				return inspectDiffInteractive(ctx, em, cfg, store, resolveOpts)
			}
			if isDiff {
				return inspectDiff(ctx, em, cfg, store, resolveOpts, diffPath)
			}

			return inspectList(ctx, em, cfg, store, resolveOpts)
		},
	}
}

// Inspect modes
// -----------------------------------------------------------------------------

func inspectList(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	resolveOpts spec.ResolveOptions,
) error {
	if err := engine.InspectList(ctx, em, cfgPath, store, resolveOpts); err != nil {
		return handleEngineError("InspectList", err)
	}
	return nil
}

func inspectDiff(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
	destPath string,
) error {
	if destPath == "" {
		paths, err := engine.InspectDiffPaths(ctx, em, cfgPath, store, opts)
		if err != nil {
			return handleEngineError("InspectDiffPaths", err)
		}
		if len(paths) == 0 {
			_, _ = fmt.Fprintln(os.Stdout, "no diffable file ops found")
			return nil
		}
		for _, p := range paths {
			_, _ = fmt.Fprintln(os.Stdout, p)
		}
		return nil
	}

	result, err := engine.InspectDiff(ctx, em, cfgPath, store, opts, destPath)
	if err != nil {
		return handleEngineError("InspectDiff", err)
	}

	tool := osutil.ResolveDiffTool()
	return osutil.RunDiffTool(ctx, tool, result.DestPath, result.Current, result.Desired)
}

func inspectDiffInteractive(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) error {
	finder := os.Getenv("SCAMPI_FUZZY_FINDER")
	if finder == "" {
		return cli.Exit(
			"--interactive requires $SCAMPI_FUZZY_FINDER to be set (e.g. fzf, sk)",
			exitUserError,
		)
	}

	paths, err := engine.InspectDiffPaths(ctx, em, cfgPath, store, opts)
	if err != nil {
		return handleEngineError("InspectDiffPaths", err)
	}
	if len(paths) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "no diffable file ops found")
		return nil
	}

	selected, err := osutil.RunFuzzyFinder(finder, paths)
	if err != nil {
		return err
	}
	if selected == "" {
		return nil
	}

	result, err := engine.InspectDiff(ctx, em, cfgPath, store, opts, selected)
	if err != nil {
		return handleEngineError("InspectDiff", err)
	}

	tool := osutil.ResolveDiffTool()
	return osutil.RunDiffTool(ctx, tool, result.DestPath, result.Current, result.Desired)
}
