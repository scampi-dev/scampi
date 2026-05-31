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

type slogLog struct{ l *slog.Logger }

func (s slogLog) Debug(ctx context.Context, msg string, args ...any) {
	s.l.DebugContext(ctx, msg, args...)
}

func (s slogLog) Info(ctx context.Context, msg string, args ...any) {
	s.l.InfoContext(ctx, msg, args...)
}

func (s slogLog) Warn(ctx context.Context, msg string, args ...any) {
	s.l.WarnContext(ctx, msg, args...)
}

func (s slogLog) Error(ctx context.Context, msg string, args ...any) {
	s.l.ErrorContext(ctx, msg, args...)
}

func main() {
	log := slogLog{slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd, err := newRootCmd(log).ExecuteContextC(ctx)
	switch {
	case err == nil:
		return
	case errors.Is(err, engine.ErrSnapshotRejected):
		log.Error(ctx, "snapshot rejected", "err", err)
		os.Exit(2)
	case errors.Is(err, engine.ErrApplyFailed):
		log.Error(ctx, "apply failed", "err", err)
		os.Exit(1)
	default:
		// Cobra-shape error (unknown command, bad args). Render the
		// resolved command's usage so the user sees the right help.
		_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n\n%s", err, cmd.UsageString())
		os.Exit(1)
	}
}

func newRootCmd(log engine.Log) *cobra.Command {
	// Silence cobra's own error+usage rendering on every command so
	// main is the single point that decides what gets shown: sentinel
	// runtime errors flow through slog; cobra-shape errors get usage
	// rendered explicitly.
	root := &cobra.Command{
		Use:           "scampi",
		Short:         "Decentralized reconciler for bare-metal infrastructure.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	apply := &cobra.Command{
		Use:           "apply <dir>",
		Short:         "Reconcile the snapshot in <dir> once.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return engine.Apply(cmd.Context(), args[0], log)
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
			return engine.Run(cmd.Context(), args[0], interval, log)
		},
	}
	run.Flags().Duration("interval", 5*time.Second, "poll interval between snapshots")

	root.AddCommand(apply, run)
	return root
}
