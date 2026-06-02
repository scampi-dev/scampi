// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

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

// actionEmitter writes lifecycle events as JSONL. log.* codes get
// filtered so the action log stays the stable machine-readable stream.
type actionEmitter struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

func newActionEmitter(dir string) (*actionEmitter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("action log dir: %w", err)
	}
	path, err := activeSegment(dir)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return &actionEmitter{f: f, enc: enc}, nil
}

// activeSegment returns the highest-numbered *.jsonl segment in dir,
// or 0001.jsonl when the dir holds none. 4-digit zero padding makes
// lexical sort match numeric sort up to 9999 segments.
func activeSegment(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return filepath.Join(dir, "0001.jsonl"), nil
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
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
