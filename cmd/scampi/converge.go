// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"

	"github.com/urfave/cli/v3"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
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

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
			}

			store := diagnostic.NewSourceStore()

			displ, cleanup := withDisplayer(opts, store)
			defer cleanup()

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)
			err := engine.Apply(ctx, em, cfg, store, resolveOpts)
			return handleEngineError("Apply", err)
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

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
			}

			store := diagnostic.NewSourceStore()

			displ, cleanup := withDisplayer(opts, store)
			defer cleanup()

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)
			err := engine.Check(ctx, em, cfg, store, resolveOpts)
			return handleEngineError("Check", err)
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

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
			}

			store := diagnostic.NewSourceStore()

			displ, cleanup := withDisplayer(opts, store)
			defer cleanup()

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)

			err := engine.Plan(ctx, em, cfg, store, resolveOpts)
			return handleEngineError("Plan", err)
		},
	}
}
