// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

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
			displ, cleanup := withDisplayer(ctx, opts, nil)
			defer cleanup()
			displ.RenderLegend()
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
