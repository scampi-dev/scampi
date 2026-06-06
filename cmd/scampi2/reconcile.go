// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/render"
)

func reconcileCmd() *cli.Command {
	var dir string
	return &cli.Command{
		Name:      "reconcile",
		Usage:     "Reconcile the snapshot in <dir> once.",
		ArgsUsage: "<dir>",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "dir", Destination: &dir},
		},
		Before: requireArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			listener, err := acquireInstanceListener(instanceAddr(cmd))
			if err != nil {
				return cli.Exit(err.Error(), 1)
			}
			defer func() { _ = listener.Close() }()

			actionLogDir, err := resolveActionLogDir(cmd)
			if err != nil {
				return err
			}

			renderer, finalize := pickReconcileEmitter(cmd)
			rerr := engine.Reconcile(ctx, engine.ReconcileConfig{
				Dir:          dir,
				ActionLogDir: actionLogDir,
				Emitter:      renderer,
			})
			finalize(rerr)
			return rerr
		},
	}
}

func pickReconcileEmitter(cmd *cli.Command) (engine.Emitter, func(error)) {
	v := resolveVerbosity(cmd)
	if cmd.String("output-format") == "json" {
		return render.NewJSONRenderer(os.Stdout, v), func(error) {}
	}
	ar := render.NewReportRenderer(
		os.Stdout,
		decideGlyphs(cmd.Bool("ascii")),
		decideColor(cmd.String("color"), os.Stdout),
		v,
	)
	return ar, ar.Finalize
}
