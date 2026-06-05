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
			r.logLine("INF", ansiGreen, args)
		}
	case engine.CodeLogWarn:
		r.logLine("WRN", ansiYellow, args)
	case engine.CodeLogError:
		r.logLine("ERR", ansiRed, args)
	case engine.CodeLogDebug:
		if r.verbosity >= VerbosityVerbose {
			r.logLine("DBG", ansiDark, args)
		}
	case engine.CodeApplyStart, engine.CodeDestroyStart, engine.CodeSnapshotReceived:
		// lifecycle wrappers; sigil lines convey the meaningful work
	}
}

func (r *RunRenderer) tickSummary(args []any) {
	duration := attrString(args, "duration")
	status := attrString(args, "status")
	ts := time.Now().Format("15:04:05.000")
	color := ansiGreen
	tag := padCol("OK", indicatorWidth)
	if status != "ok" {
		color = ansiRed
		tag = padCol("ERR", indicatorWidth)
	}
	if !r.colored {
		_, _ = fmt.Fprintf(r.out, "%s  %s  reconcile %s in %s\n", ts, tag, status, duration)
		return
	}
	_, _ = fmt.Fprintf(
		r.out, "%s%s%s  %s%s%s  %sreconcile %s%s in %s\n",
		ansiDark, ts, ansiReset,
		color, tag, ansiReset,
		ansiDim, status, ansiUndim,
		duration,
	)
}

func (r *RunRenderer) applyLine(ref *engine.Ref, args []any) {
	action := attrString(args, "action")
	switch action {
	case "create":
		r.eventLine(r.glyphs.Create, "create", ref.String(), "", ansiGreen)
	case "update":
		r.eventLine(r.glyphs.Update, "update", ref.String(), "", ansiYellow)
	case "adopt":
		r.eventLine(r.glyphs.Adopt, "adopt", ref.String(), "", ansiCyan)
	default:
		r.eventLine(r.glyphs.Update, action, ref.String(), "", ansiYellow)
	}
}

func (r *RunRenderer) failedLine(ref *engine.Ref, args []any) {
	errMsg := attrString(args, "err")
	r.eventLine(r.glyphs.Failed, "failed", ref.String(), errMsg, ansiRed)
}

func (r *RunRenderer) haltedLine(ref *engine.Ref, args []any) {
	state := attrString(args, "state")
	detail := ""
	if state != "" {
		detail = "exists (" + state + "), no adopt"
	}
	r.eventLine(r.glyphs.Halt, "halt", ref.String(), detail, ansiYellow)
}

func (r *RunRenderer) renamedLine(ref *engine.Ref, args []any) {
	from := attrString(args, "from")
	r.eventLine(r.glyphs.Rename, "rename", ref.String(), "from "+from, ansiCyan)
}

func (r *RunRenderer) destroyLine(ref *engine.Ref) {
	r.eventLine(r.glyphs.Destroy, "destroy", ref.String(), "", ansiRed)
}

func (r *RunRenderer) eventLine(glyph, label, ref, detail, color string) {
	ts := time.Now().Format("15:04:05.000")
	ind := padCol(glyph, indicatorWidth)
	if !r.colored {
		if detail != "" {
			_, _ = fmt.Fprintf(r.out, "%s  %s  %-20s  %s: %s\n", ts, ind, ref, label, detail)
		} else {
			_, _ = fmt.Fprintf(r.out, "%s  %s  %-20s  %s\n", ts, ind, ref, label)
		}
		return
	}
	if detail != "" {
		_, _ = fmt.Fprintf(
			r.out, "%s%s%s  %s%s%s  %-20s  %s%s: %s%s\n",
			ansiDark, ts, ansiReset,
			color, ind, ansiReset,
			ref,
			ansiDim, label, detail, ansiUndim,
		)
		return
	}
	_, _ = fmt.Fprintf(
		r.out, "%s%s%s  %s%s%s  %-20s  %s%s%s\n",
		ansiDark, ts, ansiReset,
		color, ind, ansiReset,
		ref,
		ansiDim, label, ansiUndim,
	)
}

func (r *RunRenderer) logLine(tag, color string, args []any) {
	msg, rest := popMsg(args)
	attrs := formatAttrs(rest, r.colored)
	ts := time.Now().Format("15:04:05.000")
	ind := padCol(tag, indicatorWidth)
	// CR+clear so the engine's shutdown announce overwrites the
	// TTY-echoed ^C in the same Write.
	prefix := ""
	if r.colored && msg == shutdownMsg {
		prefix = "\r\x1b[K"
	}
	if !r.colored {
		_, _ = fmt.Fprintf(r.out, "%s%s  %s  %s%s\n", prefix, ts, ind, msg, attrs)
		return
	}
	_, _ = fmt.Fprintf(r.out, "%s%s%s%s  %s%s%s  %s%s\n",
		prefix, ansiDark, ts, ansiReset, color, ind, ansiReset, msg, attrs)
}

const shutdownMsg = "received shutdown signal, exiting at next safe point"
