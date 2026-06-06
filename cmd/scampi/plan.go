// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/render"
)

func pickPlanEmitter() engine.Emitter {
	v := resolveVerbosity()
	if outputFormat == "json" {
		return render.NewJSONRenderer(os.Stdout, v)
	}
	return render.NewApplyRenderer(os.Stdout, decideGlyphs(), decideColor(os.Stdout), v)
}

func newPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "plan <dir>",
		Short:         "Show what reconcile would do without changing anything.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			inv, err := engine.LoadInventoryLenient(actionLogPath)
			if err != nil {
				return fmt.Errorf("action log replay: %w", err)
			}
			renderer := pickPlanEmitter()
			p, err := engine.MakePlan(cmd.Context(), engine.PlanConfig{
				Dir:       args[0],
				Inventory: inv,
				Emitter:   renderer,
			})
			if err != nil {
				return err
			}
			render.PrintPlan(os.Stdout, p, decideGlyphs(), decideColor(os.Stdout))
			return nil
		},
	}
}
