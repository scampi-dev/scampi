// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"scampi.dev/scampi/internal/engine"
)

type ReportRenderer struct {
	out       io.Writer
	glyphs    Glyphs
	colored   bool
	verbosity Verbosity
	start     time.Time
	counts    applyCounts
	rejected  bool
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

func NewReportRenderer(out io.Writer, g Glyphs, colored bool, v Verbosity) *ReportRenderer {
	return &ReportRenderer{
		out:       out,
		glyphs:    g,
		colored:   colored,
		verbosity: v,
		start:     time.Now(),
	}
}

func (*ReportRenderer) Err() error { return nil }

func (r *ReportRenderer) Emit(_ context.Context, code engine.Code, ref *engine.Ref, args ...any) {
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
	case engine.CodeLogInfo:
		if r.verbosity >= VerbosityVerbose {
			r.handleLog(code, args)
		}
	case engine.CodeLogDebug:
		if r.verbosity >= VerbosityTrace {
			r.handleLog(code, args)
		}
	case engine.CodeApplyStart, engine.CodeDestroyStart, engine.CodeSnapshotReceived:
		// lifecycle wrappers; sigil lines convey the meaningful work
	case engine.CodeLogWarn, engine.CodeLogError:
		r.handleLog(code, args)
	}
}

func (r *ReportRenderer) Finalize(err error) {
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
	label := "Reconciled"
	if r.counts.failed > 0 {
		label = "Reconcile incomplete"
	}
	if errors.Is(err, context.Canceled) {
		label = "Reconcile interrupted"
	}
	elapsed := time.Since(r.start).Round(time.Millisecond)
	if r.colored {
		_, _ = fmt.Fprintf(
			r.out,
			"\n  %s%s:%s %s in %s\n",
			AnsiBold,
			label,
			AnsiUndim,
			body,
			elapsed,
		)
	} else {
		_, _ = fmt.Fprintf(r.out, "\n  %s: %s in %s\n", label, body, elapsed)
	}
}

func (r *ReportRenderer) handleApplySuccess(ref *engine.Ref, args []any) {
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

func (r *ReportRenderer) handleApplyFailed(ref *engine.Ref, args []any) {
	errMsg := attrString(args, "err")
	r.counts.failed++
	r.writeLine(r.glyphs.Failed, "failed", ref.String(), errMsg, AnsiRed)
}

func (r *ReportRenderer) handleRenamed(ref *engine.Ref, args []any) {
	from := attrString(args, "from")
	r.counts.renamed++
	detail := "from " + from
	r.writeLine(r.glyphs.Rename, "rename", ref.String(), detail, AnsiCyan)
}

func (r *ReportRenderer) handleHalted(ref *engine.Ref, args []any) {
	state := attrString(args, "state")
	r.counts.halted++
	detail := ""
	if state != "" {
		detail = "exists (" + state + "), no adopt"
	}
	r.writeLine(r.glyphs.Halt, "halt", ref.String(), detail, AnsiYellow)
}

func (r *ReportRenderer) handleDestroySuccess(ref *engine.Ref) {
	r.counts.destroyed++
	r.writeLine(r.glyphs.Destroy, "destroy", ref.String(), "", AnsiRed)
}

func (r *ReportRenderer) handleLog(code engine.Code, args []any) {
	msg, _ := popMsg(args)
	tag := r.glyphs.Warn
	color := AnsiYellow
	if code == engine.CodeLogError {
		tag = r.glyphs.Error
		color = AnsiRed
	}
	if r.colored {
		_, _ = fmt.Fprintf(r.out, "  %s%s%s  %s\n", color, tag, AnsiReset, msg)
	} else {
		_, _ = fmt.Fprintf(r.out, "  %s  %s\n", tag, msg)
	}
}

func (r *ReportRenderer) writeLine(glyph, label, ref, detail, color string) {
	if !r.colored {
		if detail != "" {
			_, _ = fmt.Fprintf(r.out, "  %s  %-20s  %s: %s\n", glyph, ref, label, detail)
		} else {
			_, _ = fmt.Fprintf(r.out, "  %s  %-20s  %s\n", glyph, ref, label)
		}
		return
	}
	if detail != "" {
		_, _ = fmt.Fprintf(
			r.out, "  %s%s%s  %-20s  %s%s: %s%s\n",
			color, glyph, AnsiReset,
			ref,
			AnsiDim, label, detail, AnsiUndim,
		)
		return
	}
	_, _ = fmt.Fprintf(
		r.out, "  %s%s%s  %-20s  %s%s%s\n",
		color, glyph, AnsiReset,
		ref,
		AnsiDim, label, AnsiUndim,
	)
}
