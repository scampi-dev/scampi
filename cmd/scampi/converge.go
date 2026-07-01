// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"

	"github.com/urfave/cli/v3"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
)

// scampi apply
// -----------------------------------------------------------------------------

func applyCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:      "apply",
		Usage:     "Apply the desired state from a configuration file",
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file and executes the
required operations to converge the system to the desired state.

The command is idempotent: running it multiple times only applies
changes when the current state differs from the declared state.`,
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Before:                 requireArgs(1),
		Flags:                  resolveFlags(),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			pol := cliPolicy(opts)

			store := diagnostic.NewSourceStore()

			displ, cleanup := withStreamDisplayer(ctx, opts, store)
			defer cleanup()

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)
			report, err := engine.Apply(diagnostic.NewCtx(ctx, em), cfg, store, resolveOpts)
			if err != nil {
				return handleEngineError("Apply", err)
			}
			displ.RenderSummary(report, false)
			return nil
		},
	}
}

// scampi check
// -----------------------------------------------------------------------------

func checkCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:      "check",
		Usage:     "Check the current system state against a configuration file",
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file and inspects the
target system to determine which operations are already satisfied and
which would need to execute.

No changes are made to the system. Unlike 'plan', this command evaluates
the actual system state.`,
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Before:                 requireArgs(1),
		Flags:                  resolveFlags(),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			pol := cliPolicy(opts)

			store := diagnostic.NewSourceStore()

			displ, cleanup := withStreamDisplayer(ctx, opts, store)
			defer cleanup()

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)
			report, err := engine.Check(diagnostic.NewCtx(ctx, em), cfg, store, resolveOpts)
			if err != nil {
				return handleEngineError("Check", err)
			}
			displ.RenderSummary(report, true)
			return nil
		},
	}
}

// scampi plan
// -----------------------------------------------------------------------------

func planCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:      "plan",
		Usage:     "Show the execution plan for a configuration file",
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file and prints the
execution plan without applying any changes.

The plan shows the operations that would be executed by 'apply', but
does not inspect or modify the target system.`,
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Before:                 requireArgs(1),
		Flags:                  resolveFlags(),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			pol := cliPolicy(opts)

			store := diagnostic.NewSourceStore()

			displ, cleanup := withDisplayer(ctx, opts, store)
			defer cleanup()

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)

			result, err := engine.Plan(diagnostic.NewCtx(ctx, em), cfg, store, resolveOpts)
			if err != nil {
				return handleEngineError("Plan", err)
			}
			displ.RenderPlan(result)
			return nil
		},
	}
}
