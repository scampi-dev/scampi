// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"scampi.dev/scampi/internal/engine"
)

// applyRenderer is the Emitter the apply command uses in place of
// the streaming slog handler. Per-resource lifecycle events become
// sigil-prefixed lines; log.* at debug/info gets suppressed so the
// operator only sees what actually changed.
type applyRenderer struct {
	out      io.Writer
	glyphs   Glyphs
	colored  bool
	start    time.Time
	counts   applyCounts
	rejected bool
}

type applyCounts struct {
	created   int
	updated   int
	adopted   int
	renamed   int
	destroyed int
	halted    int
	failed    int
}

func newApplyRenderer(out io.Writer, g Glyphs, colored bool) *applyRenderer {
	return &applyRenderer{
		out:     out,
		glyphs:  g,
		colored: colored,
		start:   time.Now(),
	}
}

func (r *applyRenderer) Err() error { return nil }

func (r *applyRenderer) Emit(_ context.Context, code engine.Code, ref *engine.Ref, args ...any) {
	switch code {
	case engine.CodeApplySuccess:
		r.handleApplySuccess(ref, args)
	case engine.CodeApplyFailed:
		r.handleApplyFailed(ref, args)
	case engine.CodeApplyHalted:
		r.handleHalted(ref, args)
	case engine.CodeApplyRenamed:
		r.handleRenamed(ref, args)
	case engine.CodeDestroySuccess:
		r.handleDestroySuccess(ref)
	case engine.CodeDestroyFailed:
		r.handleApplyFailed(ref, args) // same failed glyph + err
	case engine.CodeSnapshotRejected:
		ts := time.Now().Format("2006-01-02 15:04:05")
		phase := attrString(args, "phase")
		errMsg := attrString(args, "err")
		_ = renderSnapshotRejected(r.out, ts, phase, errMsg, r.colored)
		r.rejected = true
	case engine.CodeApplyStart, engine.CodeDestroyStart,
		engine.CodeSnapshotReceived,
		engine.CodeLogDebug, engine.CodeLogInfo:
		// suppressed at default verbosity
	case engine.CodeLogWarn, engine.CodeLogError:
		r.handleLog(code, args)
	}
}

func (r *applyRenderer) Finalize(_ error) {
	if r.rejected {
		// renderSnapshotRejected already told the operator everything.
		return
	}
	bits := []string{}
	if r.counts.created > 0 {
		bits = append(bits, fmt.Sprintf("%d created", r.counts.created))
	}
	if r.counts.updated > 0 {
		bits = append(bits, fmt.Sprintf("%d updated", r.counts.updated))
	}
	if r.counts.adopted > 0 {
		bits = append(bits, fmt.Sprintf("%d adopted", r.counts.adopted))
	}
	if r.counts.renamed > 0 {
		bits = append(bits, fmt.Sprintf("%d renamed", r.counts.renamed))
	}
	if r.counts.destroyed > 0 {
		bits = append(bits, fmt.Sprintf("%d destroyed", r.counts.destroyed))
	}
	if r.counts.halted > 0 {
		bits = append(bits, fmt.Sprintf("%d halted", r.counts.halted))
	}
	if r.counts.failed > 0 {
		bits = append(bits, fmt.Sprintf("%d failed", r.counts.failed))
	}
	body := strings.Join(bits, ", ")
	if body == "" {
		body = "nothing to do"
	}
	label := "Applied"
	if r.counts.failed > 0 {
		label = "Apply incomplete"
	}
	elapsed := time.Since(r.start).Round(time.Millisecond)
	if r.colored {
		_, _ = fmt.Fprintf(r.out, "\n  %s%s:%s %s in %s\n", ansiBold, label, ansiUndim, body, elapsed)
	} else {
		_, _ = fmt.Fprintf(r.out, "\n  %s: %s in %s\n", label, body, elapsed)
	}
}

func (r *applyRenderer) handleApplySuccess(ref *engine.Ref, args []any) {
	action := attrString(args, "action")
	var glyph, label string
	switch action {
	case "create":
		glyph, label = r.glyphs.Create, "create"
		r.counts.created++
	case "update":
		glyph, label = r.glyphs.Update, "update"
		r.counts.updated++
	case "adopt":
		glyph, label = r.glyphs.Adopt, "adopt"
		r.counts.adopted++
	default:
		// Unknown action: still surface the success but with a neutral sigil.
		glyph, label = r.glyphs.Update, action
		r.counts.updated++
	}
	r.writeLine(glyph, label, ref.String(), "", planColor(label))
}

func (r *applyRenderer) handleApplyFailed(ref *engine.Ref, args []any) {
	errMsg := attrString(args, "err")
	r.counts.failed++
	r.writeLine(r.glyphs.Failed, "failed", ref.String(), errMsg, ansiRed)
}

func (r *applyRenderer) handleRenamed(ref *engine.Ref, args []any) {
	from := attrString(args, "from")
	r.counts.renamed++
	detail := "from " + from
	r.writeLine(r.glyphs.Rename, "rename", ref.String(), detail, ansiCyan)
}

func (r *applyRenderer) handleHalted(ref *engine.Ref, args []any) {
	state := attrString(args, "state")
	r.counts.halted++
	detail := ""
	if state != "" {
		detail = "exists (" + state + "), no adopt"
	}
	r.writeLine(r.glyphs.Halt, "halt", ref.String(), detail, ansiYellow)
}

func (r *applyRenderer) handleDestroySuccess(ref *engine.Ref) {
	r.counts.destroyed++
	r.writeLine(r.glyphs.Destroy, "destroy", ref.String(), "", ansiRed)
}

func (r *applyRenderer) handleLog(code engine.Code, args []any) {
	msg, _ := popMsg(args)
	tag := "WRN"
	color := ansiYellow
	if code == engine.CodeLogError {
		tag = "ERR"
		color = ansiRed
	}
	ind := padCol(tag, indicatorWidth)
	if r.colored {
		_, _ = fmt.Fprintf(r.out, "  %s%s%s  %s\n", color, ind, ansiReset, msg)
	} else {
		_, _ = fmt.Fprintf(r.out, "  %s  %s\n", ind, msg)
	}
}

func (r *applyRenderer) writeLine(glyph, label, ref, detail, color string) {
	ind := padCol(glyph, indicatorWidth)
	if !r.colored {
		if detail != "" {
			_, _ = fmt.Fprintf(r.out, "  %s  %-20s  %s: %s\n", ind, ref, label, detail)
		} else {
			_, _ = fmt.Fprintf(r.out, "  %s  %-20s  %s\n", ind, ref, label)
		}
		return
	}
	if detail != "" {
		_, _ = fmt.Fprintf(r.out, "  %s%s%s  %-20s  %s%s: %s%s\n",
			color, ind, ansiReset,
			ref,
			ansiDim, label, detail, ansiUndim,
		)
		return
	}
	_, _ = fmt.Fprintf(r.out, "  %s%s%s  %-20s  %s%s%s\n",
		color, ind, ansiReset,
		ref,
		ansiDim, label, ansiUndim,
	)
}

func attrString(args []any, key string) string {
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok || k != key {
			continue
		}
		v := args[i+1]
		if e, ok := v.(error); ok {
			return e.Error()
		}
		return fmt.Sprint(v)
	}
	return ""
}
