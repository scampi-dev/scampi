// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"scampi.dev/scampi/internal/engine"
)

var errUsage = errors.New("usage: scampi {apply|run} <dir>")

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
	err := dispatch(ctx, os.Args[1:], log)
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
		log.Error(ctx, "scampi failed", "err", err)
		os.Exit(1)
	}
}

func dispatch(ctx context.Context, args []string, log engine.Log) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "apply":
		return cmdApply(ctx, args[1:], log)
	case "run":
		return cmdRun(ctx, args[1:], log)
	default:
		return errUsage
	}
}

func cmdApply(ctx context.Context, args []string, log engine.Log) error {
	fset := flag.NewFlagSet("apply", flag.ContinueOnError)
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() != 1 {
		return errUsage
	}
	return engine.Apply(ctx, fset.Arg(0), log)
}

func cmdRun(ctx context.Context, args []string, log engine.Log) error {
	fset := flag.NewFlagSet("run", flag.ContinueOnError)
	interval := fset.Duration("interval", 5*time.Second, "poll interval between snapshots")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() != 1 {
		return errUsage
	}
	return engine.Run(ctx, fset.Arg(0), *interval, log)
}
