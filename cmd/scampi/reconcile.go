// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"os"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/render"
)

func pickReconcileEmitter() (engine.Emitter, func(error)) {
	v := resolveVerbosity()
	if outputFormat == "json" {
		return render.NewJSONRenderer(os.Stdout, v), func(error) {}
	}
	ar := render.NewReportRenderer(os.Stdout, decideGlyphs(), decideColor(os.Stdout), v)
	return ar, ar.Finalize
}

func newReconcileCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "reconcile <dir>",
		Short:         "Reconcile the snapshot in <dir> once.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := acquireInstanceListener()
			if err != nil {
				return err
			}
			instance = l
			renderer, finalize := pickReconcileEmitter()
			err = engine.Reconcile(cmd.Context(), engine.ReconcileConfig{
				Dir:          args[0],
				ActionLogDir: actionLogPath,
				Emitter:      renderer,
			})
			finalize(err)
			return err
		},
	}
}
