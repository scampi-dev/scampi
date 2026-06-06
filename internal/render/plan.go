// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"scampi.dev/scampi/internal/engine"
)

type planEntry struct {
	ref   engine.Ref
	glyph string
	label string
}

func collectPlanEntries(p *engine.Plan, g Glyphs) []planEntry {
	var entries []planEntry
	add := func(refs []engine.Ref, glyph, label string) {
		for _, r := range refs {
			entries = append(entries, planEntry{ref: r, glyph: glyph, label: label})
		}
	}
	add(p.Create, g.Create, "create")
	add(p.Update, g.Update, "update")
	add(p.Adopt, g.Adopt, "adopt")
	add(p.Halt, g.Halt, "halt")
	add(p.Destroy, g.Destroy, "destroy")
	add(p.InSync, g.InSync, "in sync")
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ref.String() < entries[j].ref.String()
	})
	return entries
}

func PrintPlan(w io.Writer, p *engine.Plan, g Glyphs, colored bool) {
	entries := collectPlanEntries(p, g)
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(w, "plan: empty")
		return
	}
	maxRef := 0
	for _, e := range entries {
		if l := len(e.ref.String()); l > maxRef {
			maxRef = l
		}
	}
	_, _ = fmt.Fprintln(w)
	for _, e := range entries {
		writePlanEntry(w, e, maxRef, colored)
	}
	_, _ = fmt.Fprintln(w)
	writePlanSummary(w, p, colored)
}

func writePlanEntry(w io.Writer, e planEntry, refWidth int, colored bool) {
	ref := e.ref.String()
	ind := padCol(e.glyph, indicatorWidth)
	if !colored {
		_, _ = fmt.Fprintf(w, "  %s  %-*s  %s\n", ind, refWidth, ref, e.label)
		return
	}
	col := planColor(e.label)
	_, _ = fmt.Fprintf(
		w, "  %s%s%s  %-*s  %s%s%s\n",
		col, ind, AnsiReset,
		refWidth, ref,
		AnsiDim, e.label, AnsiUndim,
	)
}

func planColor(label string) string {
	switch label {
	case "create":
		return AnsiGreen
	case "update", "halt":
		return AnsiYellow
	case "destroy", "failed":
		return AnsiRed
	case "adopt", "rename":
		return AnsiCyan
	case "in sync":
		return AnsiDark
	}
	return ""
}

func writePlanSummary(w io.Writer, p *engine.Plan, colored bool) {
	parts := []struct {
		label string
		n     int
	}{
		{"create", len(p.Create)},
		{"update", len(p.Update)},
		{"adopt", len(p.Adopt)},
		{"halt", len(p.Halt)},
		{"destroy", len(p.Destroy)},
		{"in sync", len(p.InSync)},
	}
	var bits []string
	for _, p := range parts {
		if p.n == 0 {
			continue
		}
		bits = append(bits, fmt.Sprintf("%d %s", p.n, p.label))
	}
	body := strings.Join(bits, ", ")
	if colored {
		_, _ = fmt.Fprintf(w, "  %sPlan:%s %s\n", AnsiBold, AnsiUndim, body)
	} else {
		_, _ = fmt.Fprintf(w, "  Plan: %s\n", body)
	}
}
