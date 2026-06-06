// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"context"
	"fmt"
	"io"
	"time"

	"scampi.dev/scampi/internal/engine"
)

type RunRenderer struct {
	out        io.Writer
	glyphs     Glyphs
	colored    bool
	verbosity  Verbosity
	tickEvents int
}

func NewRunRenderer(out io.Writer, g Glyphs, colored bool, v Verbosity) *RunRenderer {
	return &RunRenderer{out: out, glyphs: g, colored: colored, verbosity: v}
}

func (*RunRenderer) Err() error { return nil }

func (r *RunRenderer) Emit(_ context.Context, code engine.Code, ref *engine.Ref, args ...any) {
	switch code {
	case engine.CodeApplySuccess:
		r.applyLine(ref, args)
		r.tickEvents++
	case engine.CodeApplyFailed, engine.CodeDestroyFailed:
		r.failedLine(ref, args)
		r.tickEvents++
	case engine.CodeApplyHalted:
		r.haltedLine(ref, args)
		r.tickEvents++
	case engine.CodeApplyRenamed:
		r.renamedLine(ref, args)
		r.tickEvents++
	case engine.CodeDestroySuccess:
		r.destroyLine(ref)
		r.tickEvents++
	case engine.CodeSnapshotRejected:
		ts := time.Now().Format("2006-01-02 15:04:05")
		phase := attrString(args, "phase")
		errMsg := attrString(args, "err")
		_ = renderSnapshotRejected(r.out, ts, phase, errMsg, r.colored)
		r.tickEvents++
	case engine.CodeTickComplete:
		if r.tickEvents > 0 {
			r.tickSummary(args)
		}
		r.tickEvents = 0
	case engine.CodeLogInfo:
		if r.verbosity >= VerbosityDefault {
			r.logLine(r.glyphs.Info, AnsiGreen, args)
		}
	case engine.CodeLogWarn:
		r.logLine(r.glyphs.Warn, AnsiYellow, args)
	case engine.CodeLogError:
		r.logLine(r.glyphs.Error, AnsiRed, args)
	case engine.CodeLogDebug:
		if r.verbosity >= VerbosityVerbose {
			r.logLine(r.glyphs.Debug, AnsiDark, args)
		}
	case engine.CodeMeshUp:
		if r.verbosity >= VerbosityDefault {
			r.logLine(r.glyphs.Info, AnsiGreen, append([]any{"msg", "mesh up"}, args...))
		}
	case engine.CodeMeshDown:
		if r.verbosity >= VerbosityDefault {
			r.logLine(r.glyphs.Info, AnsiGreen, append([]any{"msg", "mesh down"}, args...))
		}
	case engine.CodeMeshPeerJoined:
		if r.verbosity >= VerbosityDefault {
			r.logLine(r.glyphs.Info, AnsiGreen, append([]any{"msg", "peer joined"}, args...))
		}
	case engine.CodeMeshPeerLeft:
		if r.verbosity >= VerbosityDefault {
			r.logLine(r.glyphs.Info, AnsiGreen, append([]any{"msg", "peer left"}, args...))
		}
	case engine.CodeMeshPeerUpdated:
		if r.verbosity >= VerbosityVerbose {
			r.logLine(r.glyphs.Debug, AnsiDark, append([]any{"msg", "peer updated"}, args...))
		}
	case engine.CodeMeshUnavailable:
		r.logLine(
			r.glyphs.Warn, AnsiYellow,
			append([]any{"msg", "mesh unavailable; running engine-only"}, args...),
		)
	case engine.CodeApplyStart, engine.CodeDestroyStart, engine.CodeSnapshotReceived:
		// lifecycle wrappers; sigil lines convey the meaningful work
	}
}

func (r *RunRenderer) tickSummary(args []any) {
	duration := attrString(args, "duration")
	status := attrString(args, "status")
	ts := time.Now().Format("15:04:05.000")
	color := AnsiGreen
	tag := r.glyphs.OK
	if status != "ok" {
		color = AnsiRed
		tag = r.glyphs.Error
	}
	if !r.colored {
		_, _ = fmt.Fprintf(r.out, "%s  %s  reconcile %s in %s\n", ts, tag, status, duration)
		return
	}
	_, _ = fmt.Fprintf(
		r.out, "%s%s%s  %s%s%s  %sreconcile %s%s in %s\n",
		AnsiDark, ts, AnsiReset,
		color, tag, AnsiReset,
		AnsiDim, status, AnsiUndim,
		duration,
	)
}

func (r *RunRenderer) applyLine(ref *engine.Ref, args []any) {
	action := attrString(args, "action")
	switch action {
	case "create":
		r.eventLine(r.glyphs.Create, "create", ref.String(), "", AnsiGreen)
	case "update":
		r.eventLine(r.glyphs.Update, "update", ref.String(), "", AnsiYellow)
	case "adopt":
		r.eventLine(r.glyphs.Adopt, "adopt", ref.String(), "", AnsiCyan)
	default:
		r.eventLine(r.glyphs.Update, action, ref.String(), "", AnsiYellow)
	}
}

func (r *RunRenderer) failedLine(ref *engine.Ref, args []any) {
	errMsg := attrString(args, "err")
	r.eventLine(r.glyphs.Failed, "failed", ref.String(), errMsg, AnsiRed)
}

func (r *RunRenderer) haltedLine(ref *engine.Ref, args []any) {
	state := attrString(args, "state")
	detail := ""
	if state != "" {
		detail = "exists (" + state + "), no adopt"
	}
	r.eventLine(r.glyphs.Halt, "halt", ref.String(), detail, AnsiYellow)
}

func (r *RunRenderer) renamedLine(ref *engine.Ref, args []any) {
	from := attrString(args, "from")
	r.eventLine(r.glyphs.Rename, "rename", ref.String(), "from "+from, AnsiCyan)
}

func (r *RunRenderer) destroyLine(ref *engine.Ref) {
	r.eventLine(r.glyphs.Destroy, "destroy", ref.String(), "", AnsiRed)
}

func (r *RunRenderer) eventLine(glyph, label, ref, detail, color string) {
	ts := time.Now().Format("15:04:05.000")
	if !r.colored {
		if detail != "" {
			_, _ = fmt.Fprintf(r.out, "%s  %s  %-20s  %s: %s\n", ts, glyph, ref, label, detail)
		} else {
			_, _ = fmt.Fprintf(r.out, "%s  %s  %-20s  %s\n", ts, glyph, ref, label)
		}
		return
	}
	if detail != "" {
		_, _ = fmt.Fprintf(
			r.out, "%s%s%s  %s%s%s  %-20s  %s%s: %s%s\n",
			AnsiDark, ts, AnsiReset,
			color, glyph, AnsiReset,
			ref,
			AnsiDim, label, detail, AnsiUndim,
		)
		return
	}
	_, _ = fmt.Fprintf(
		r.out, "%s%s%s  %s%s%s  %-20s  %s%s%s\n",
		AnsiDark, ts, AnsiReset,
		color, glyph, AnsiReset,
		ref,
		AnsiDim, label, AnsiUndim,
	)
}

func (r *RunRenderer) logLine(tag, color string, args []any) {
	msg, rest := popMsg(args)
	attrs := formatAttrs(rest, r.colored)
	ts := time.Now().Format("15:04:05.000")
	// CR+clear so the engine's shutdown announce overwrites the
	// TTY-echoed ^C in the same Write.
	prefix := ""
	if r.colored && msg == shutdownMsg {
		prefix = "\r\x1b[K"
	}
	if !r.colored {
		_, _ = fmt.Fprintf(r.out, "%s%s  %s  %s%s\n", prefix, ts, tag, msg, attrs)
		return
	}
	_, _ = fmt.Fprintf(r.out, "%s%s%s%s  %s%s%s  %s%s\n",
		prefix, AnsiDark, ts, AnsiReset, color, tag, AnsiReset, msg, attrs)
}

const shutdownMsg = "received shutdown signal, exiting at next safe point"
