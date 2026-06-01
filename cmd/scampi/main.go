// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
)

// slogEmitter renders every emission through slog: log.* codes map
// to the matching level and pull "msg" out as the message; lifecycle
// codes use the code itself as the message.
type slogEmitter struct {
	l *slog.Logger
}

func (s slogEmitter) Emit(ctx context.Context, code engine.Code, ref *engine.Ref, args ...any) {
	var (
		msg  string
		rest []any
	)
	if engine.IsLogCode(code) {
		msg, rest = popMsg(args)
	} else {
		msg = string(code)
		rest = args
	}
	kv := make([]any, 0, len(rest)+4)
	kv = append(kv, "code", string(code))
	if ref != nil {
		kv = append(kv, "ref", ref.String())
	}
	kv = append(kv, rest...)
	s.l.Log(ctx, slogLevel(code), msg, kv...)
}

func slogLevel(c engine.Code) slog.Level {
	switch c {
	case engine.CodeLogDebug:
		return slog.LevelDebug
	case engine.CodeLogWarn:
		return slog.LevelWarn
	case engine.CodeLogError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// popMsg extracts the leading "msg":<string> pair injected by the
// Log struct's Debug/Info/Warn/Error helpers.
func popMsg(args []any) (string, []any) {
	if len(args) >= 2 {
		if k, ok := args[0].(string); ok && k == "msg" {
			if s, ok := args[1].(string); ok {
				return s, args[2:]
			}
		}
	}
	return "", args
}

// actionEmitter writes lifecycle events as JSONL. log.* codes get
// filtered so the action log stays the stable machine-readable stream.
type actionEmitter struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

func newActionEmitter(path string) (*actionEmitter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return &actionEmitter{f: f, enc: enc}, nil
}

func (a *actionEmitter) Close() error { return a.f.Close() }

func (a *actionEmitter) Emit(_ context.Context, code engine.Code, ref *engine.Ref, args ...any) {
	if engine.IsLogCode(code) {
		return
	}
	rec := map[string]any{"ts": time.Now(), "code": string(code)}
	if ref != nil {
		rec["ref"] = ref.String()
	}
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok {
			continue
		}
		rec[k] = args[i+1]
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Best-effort: write errors swallowed, propagating would force
	// every caller into error-handling for telemetry.
	_ = a.enc.Encode(rec)
}

// fanoutEmitter fans every emission out to each Emitter in order.
type fanoutEmitter []engine.Emitter

func (f fanoutEmitter) Emit(ctx context.Context, code engine.Code, ref *engine.Ref, args ...any) {
	for _, e := range f {
		e.Emit(ctx, code, ref, args...)
	}
}

func main() {
	base := slogEmitter{l: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	baseLog := engine.NewLog(base)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, closeFn := newRootCmd(base)
	cmd, err := root.ExecuteContextC(ctx)
	_ = closeFn()
	switch {
	case err == nil:
		return
	case errors.Is(err, engine.ErrSnapshotRejected):
		baseLog.Error(ctx, "snapshot rejected", "err", err)
		os.Exit(2)
	case errors.Is(err, engine.ErrApplyFailed):
		baseLog.Error(ctx, "apply failed", "err", err)
		os.Exit(1)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n\n%s", err, cmd.UsageString())
		os.Exit(1)
	}
}

func newRootCmd(base engine.Emitter) (*cobra.Command, func() error) {
	var (
		actionLogPath string
		actEm         *actionEmitter
	)
	pickLog := func() engine.Log {
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
