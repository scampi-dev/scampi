// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/mattn/go-isatty"

	"scampi.dev/scampi/internal/engine"
)

// Basic 8/16 ANSI escapes used by slogHandler. Stay in this palette
// so the operator's terminal theme decides the actual shades.
const (
	ansiDim    = "\x1b[2m"
	ansiUndim  = "\x1b[22m"
	ansiBold   = "\x1b[1m"
	ansiDark   = "\x1b[90m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiReset  = "\x1b[39m"
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
	kv := make([]any, 0, len(rest)+2)
	if ref != nil {
		kv = append(kv, "ref", ref.String())
	}
	kv = append(kv, rest...)
	s.l.Log(ctx, slogLevel(code), msg, kv...)
}

func (slogEmitter) Err() error { return nil }

func slogLevel(c engine.Code) slog.Level {
	switch c {
	case engine.CodeLogDebug:
		return slog.LevelDebug
	case engine.CodeLogWarn, engine.CodeSnapshotRejected, engine.CodeApplyFailed:
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

// slogHandler renders each slog record as a single line:
//
//	TIME LVL message key=value ...
//
// When colored: console-slog-ish styling. Debug lines are fully dim
// (except cyan attr keys); Info is plain text with a green level
// keyword and dim timestamp; Warn / Error get bold yellow / red
// messages. Basic 8/16 ANSI only so the operator's terminal theme
// decides actual shades.
type slogHandler struct {
	out     io.Writer
	colored bool
	level   slog.Level
}

func (h *slogHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *slogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *slogHandler) WithGroup(string) slog.Handler      { return h }

func (h *slogHandler) Handle(_ context.Context, r slog.Record) error {
	ts := r.Time.Format("2006-01-02 15:04:05")
	lvl := levelStr(r.Level)

	attrs := ""
	r.Attrs(func(a slog.Attr) bool {
		if h.colored {
			attrs += fmt.Sprintf(" %s%s=%s%s", ansiCyan, a.Key, ansiReset, a.Value)
		} else {
			attrs += fmt.Sprintf(" %s=%s", a.Key, a.Value)
		}
		return true
	})

	var line string
	switch {
	case !h.colored:
		line = fmt.Sprintf("%s %s %s%s\n", ts, lvl, r.Message, attrs)
	case r.Level <= slog.LevelDebug:
		// Whole-line dim. Cyan attr keys still pop; their value
		// returns to default fg (dim attr keeps it muted).
		line = fmt.Sprintf("%s%s %s %s%s%s\n", ansiDim, ts, lvl, r.Message, attrs, ansiUndim)
	default:
		levelTag, msg := levelTagAndMsg(r.Level, r.Message)
		line = fmt.Sprintf("%s%s%s %s %s%s\n", ansiDark, ts, ansiReset, levelTag, msg, attrs)
	}
	_, err := h.out.Write([]byte(line))
	return err
}

// levelTagAndMsg returns the styled level keyword and message for
// the per-element (non-debug) rendering path.
func levelTagAndMsg(l slog.Level, msg string) (string, string) {
	switch {
	case l >= slog.LevelError:
		return ansiRed + "ERR" + ansiReset, ansiBold + ansiRed + msg + ansiReset + ansiUndim
	case l >= slog.LevelWarn:
		return ansiYellow + "WRN" + ansiReset, ansiBold + ansiYellow + msg + ansiReset + ansiUndim
	default: // info
		return ansiGreen + "INF" + ansiReset, msg
	}
}

func levelStr(l slog.Level) string {
	switch {
	case l <= slog.LevelDebug:
		return "DBG"
	case l >= slog.LevelError:
		return "ERR"
	case l >= slog.LevelWarn:
		return "WRN"
	default:
		return "INF"
	}
}

func decideColor(mode string, w *os.File) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default: // auto and any unknown value
		return isatty.IsTerminal(w.Fd())
	}
}

// fanoutEmitter fans every emission out to each Emitter in order.
type fanoutEmitter []engine.Emitter

func (f fanoutEmitter) Emit(ctx context.Context, code engine.Code, ref *engine.Ref, args ...any) {
	for _, e := range f {
		e.Emit(ctx, code, ref, args...)
	}
}

// Err returns the first non-nil sticky error across the fanned-out
// sinks. The action log's sticky failure shows up here first; slog
// always reports nil.
func (f fanoutEmitter) Err() error {
	for _, e := range f {
		if err := e.Err(); err != nil {
			return err
		}
	}
	return nil
}
