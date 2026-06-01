// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, closeFn := newRootCmd()
	cmd, err := root.ExecuteContextC(ctx)
	_ = closeFn()
	switch {
	case err == nil:
		return
	case errors.Is(err, engine.ErrSnapshotRejected):
		// Engine already emitted snapshot.rejected via the log; we
		// just translate it to an exit code here.
		os.Exit(2)
	case errors.Is(err, engine.ErrApplyFailed):
		os.Exit(1)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n\n%s", err, cmd.UsageString())
		os.Exit(1)
	}
}

func newRootCmd() (*cobra.Command, func() error) {
	var (
		actionLogPath string
		colorMode     string
		actEm         *actionEmitter
	)
	pickLog := func() engine.Log {
		base := slogEmitter{l: slog.New(&slogHandler{
			out:     os.Stderr,
			colored: decideColor(colorMode, os.Stderr),
			level:   slog.LevelDebug,
		})}
		if actEm == nil {
			return engine.NewLog(base)
		}
		return engine.NewLog(fanoutEmitter{base, actEm})
	}

	root := &cobra.Command{
		Use:           "scampi",
		Short:         "Decentralized reconciler for bare-metal infrastructure.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if actionLogPath == "" {
				return nil
			}
			ae, err := newActionEmitter(actionLogPath)
			if err != nil {
				return fmt.Errorf("action log: %w", err)
			}
			actEm = ae
			return nil
		},
	}
	root.PersistentFlags().StringVar(&actionLogPath, "action-log", "",
		"path to a JSONL action log (lifecycle events only; append-only)")
	root.PersistentFlags().StringVar(&colorMode, "color", "auto",
		"colored output: auto|always|never")

	apply := &cobra.Command{
		Use:           "apply <dir>",
		Short:         "Reconcile the snapshot in <dir> once.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return engine.Apply(cmd.Context(), args[0], pickLog())
		},
	}

	run := &cobra.Command{
		Use:           "run <dir>",
		Short:         "Watch <dir> and reconcile on every change.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			interval, _ := cmd.Flags().GetDuration("interval")
			return engine.Run(cmd.Context(), args[0], interval, pickLog())
		},
	}
	run.Flags().Duration("interval", 5*time.Second, "poll interval between snapshots")

	root.AddCommand(apply, run)
	return root, func() error {
		if actEm == nil {
			return nil
		}
		return actEm.Close()
	}
}
