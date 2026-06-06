// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/render"
)

func newPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "plan <dir>",
		Short:         "Show what apply would do without changing anything.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := engine.LoadInventoryLenient(actionLogPath)
			if err != nil {
				return fmt.Errorf("action log replay: %w", err)
			}
			inv = loaded
			renderer, _ := pickApplyEmitter()
			p, err := engine.MakePlan(cmd.Context(), engine.PlanConfig{
				Dir:       args[0],
				Inventory: inv,
				Log:       engine.NewLog(renderer),
			})
			if err != nil {
				return err
			}
			render.PrintPlan(os.Stdout, p, decideGlyphs(), decideColor(os.Stdout))
			return nil
		},
	}
}
