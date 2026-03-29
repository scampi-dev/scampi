// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/errs"
)

// scampi index
// -----------------------------------------------------------------------------

func indexCmd() *cli.Command {
	return &cli.Command{
		Name:      "index",
		Usage:     "List available steps and their documentation",
		ArgsUsage: "[step]",
		Description: `Prints the index of steps supported by the engine.

Without arguments, the command lists all available steps with a short
description. When a step name is provided, detailed documentation is
shown, including fields, behavior, and examples.`,
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Before:                 requireMaxArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
			}

			displ, cleanup := withDisplayer(opts, nil)
			defer cleanup()

			em := diagnostic.NewEmitter(pol, displ)
			args := cmd.Args()

			var err error
			if args.Len() == 0 {
				err = engine.IndexAll(ctx, em)
			} else {
				err = engine.IndexStep(ctx, args.First(), em)
			}

			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					// Engine violated its contract: unexpected error
					panic(errs.BUG("engine.Index returned unexpected error: %w", err))
				}

				return cli.Exit("", exitUserError)
			}

			return nil
		},
	}
}

// scampi legend
// -----------------------------------------------------------------------------

func legendCmd() *cli.Command {
	return &cli.Command{
		Name:  "legend",
		Usage: "Show the CLI visual language reference",
		Description: `Prints a reference card for glyphs, plan structure,
and color semantics used in scampi CLI output.`,
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)
			displ, cleanup := withDisplayer(opts, nil)
			defer cleanup()
			displ.EmitLegend()
			return nil
		},
	}
}

// scampi version
// -----------------------------------------------------------------------------

func versionCmd() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print the scampi version",
		Action: func(_ context.Context, _ *cli.Command) error {
			_, _ = fmt.Println("scampi " + version)
			return nil
		},
	}
}
