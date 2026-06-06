// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/render"
)

func planCmd() *cli.Command {
	var dir string
	return &cli.Command{
		Name:      "plan",
		Usage:     "Show what reconcile would do without changing anything.",
		ArgsUsage: "<dir>",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "dir", Destination: &dir},
		},
		Before: requireArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			actionLogDir, err := resolveActionLogDir(cmd)
			if err != nil {
				return err
			}
			inv, err := engine.LoadInventoryLenient(actionLogDir)
			if err != nil {
				return fmt.Errorf("action log replay: %w", err)
			}
			renderer := pickPlanEmitter(cmd)
			p, err := engine.MakePlan(ctx, engine.PlanConfig{
				Dir:       dir,
				Inventory: inv,
				Emitter:   renderer,
			})
			if err != nil {
				return err
			}
			render.PrintPlan(
				os.Stdout, p,
				decideGlyphs(cmd.Bool("ascii")),
				decideColor(cmd.String("color"), os.Stdout),
			)
			return nil
		},
	}
}

func pickPlanEmitter(cmd *cli.Command) engine.Emitter {
	v := resolveVerbosity(cmd)
	if cmd.String("output-format") == "json" {
		return render.NewJSONRenderer(os.Stdout, v)
	}
	return render.NewReportRenderer(
		os.Stdout,
		decideGlyphs(cmd.Bool("ascii")),
		decideColor(cmd.String("color"), os.Stdout),
		v,
	)
}
