// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/render/ansi"
	"scampi.dev/scampi/internal/render/dag"
	"scampi.dev/scampi/internal/render/layout"
	"scampi.dev/scampi/internal/render/template"
	"scampi.dev/scampi/internal/signal"
)

// gap is the space between the content block and the structure block (deps +
// parallel bracket).
const gap = 2

type planRenderer struct {
	glyphs    glyphSet
	width     int
	verbosity signal.Verbosity
	fmt       *formatter
}

func newPlanRenderer(glyphs glyphSet, width int, verbosity signal.Verbosity, f *formatter) *planRenderer {
	return &planRenderer{glyphs: glyphs, width: width, verbosity: verbosity, fmt: f}
}

// planLine is one rendered row. Its parts degrade independently as the terminal
// narrows: the op detail elides first, the action desc drops next, and the
// gutter + id ("[N] kind" / "step.name") + structure are protected.
type planLine struct {
	gutter   string      // pre-colored rail/marker; always rendered in full
	id       string      // pre-colored core label: "[N] kind" or "step.name"
	desc     string      // pre-colored action desc "› ..."; dropped on a narrow term
	detail   *layout.Col // elidable trailing detail (op parenthetical / op count)
	isHeader bool
}

type actionEntry struct {
	lines      []planLine
	layerSize  int
	posInLayer int
	deps       []int
}

type planLayout struct {
	maxContentW int      // widest line, natural (gutter+id+desc+detail)
	maxLabelW   int      // widest gutter+id+desc (no detail)
	maxCoreW    int      // widest gutter+id (the protected core)
	depsStrs    []string // deps string per action (header)
	maxDepsW    int      // widest deps across all actions
	maxParDepsW int      // widest deps among parallel actions (bracket offset)
	hasParallel bool
	tooNarrow   bool // core + structure couldn't fit; warn
	minWidth    int  // columns needed when too narrow
}

func (p *planRenderer) style(c ansi.ANSI) func(string) string {
	return func(s string) string { return p.fmt.fmtMsg(c, s) }
}

func (p *planRenderer) renderPlan(d result.PlanDetail) []renderEvent {
	var out []renderEvent
	out = append(out, renderEvent{stream: streamOut, line: ""})

	hdr := p.fmt.fmtMsg(colPlanRail, p.glyphs.planStart)
	hdr += p.fmt.fmtMsg(colPlanHeader, " execution plan")
	if d.DeployID != "" {
		hdr += p.fmt.fmtfMsg(colPlanHeader, ": %s", d.DeployID)
		if d.DeployDesc != "" {
			hdr += p.fmt.fmtfMsg(colPlanHeader, " %s %s", p.glyphs.actionKindSep, d.DeployDesc)
		}
	}
	out = append(out, renderEvent{stream: streamOut, line: hdr})

	planDAG := dag.Build(d)
	actions := p.buildActionEntries(planDAG)
	ly := p.measureLayout(actions)
	out = p.renderActions(out, actions, &ly)

	if ly.tooNarrow {
		out = append(out, p.narrowWarning(ly.minWidth))
	}

	out = append(out, renderEvent{stream: streamOut, line: p.fmt.fmtMsg(colPlanRail, p.glyphs.planEnd)})
	out = append(out, renderEvent{stream: streamOut, line: ""})
	return out
}

// buildActionEntries
// -----------------------------------------------------------------------------

func (p *planRenderer) buildActionEntries(planDAG dag.PlanDAG) []actionEntry {
	maxIndex := 0
	for _, layer := range planDAG.ActionLayers {
		for _, act := range layer {
			if act.Index > maxIndex {
				maxIndex = act.Index
			}
		}
	}
	indexWidth := digits10(maxIndex)
	v := p.verbosity

	var actions []actionEntry
	for _, layer := range planDAG.ActionLayers {
		for posInLayer, act := range layer {
			ae := actionEntry{layerSize: len(layer), posInLayer: posInLayer, deps: act.DependsOn}
			if v == signal.Quiet {
				ae.lines = []planLine{p.quietLine(act, indexWidth)}
			} else {
				ae.lines = p.verboseLines(act, indexWidth, v)
			}
			actions = append(actions, ae)
		}
	}
	return actions
}

func (p *planRenderer) idStr(idx, indexWidth int, kind string) string {
	return p.fmt.fmtfMsg(colActionKind, " [%*d]%s", indexWidth, idx, kindSuffix(kind))
}

func (p *planRenderer) descStr(desc string) string {
	if desc == "" {
		return ""
	}
	return p.fmt.fmtfMsg(colActionDesc, " %s %s", p.glyphs.actionKindSep, desc)
}

func (p *planRenderer) quietLine(act dag.Action, indexWidth int) planLine {
	nOps := 0
	for _, l := range act.Layers {
		nOps += len(l)
	}
	return planLine{
		gutter:   p.railGutter(p.glyphs.actionStartCollapsed, colActionKind),
		id:       p.idStr(act.Index, indexWidth, act.Kind),
		desc:     p.descStr(act.Desc),
		detail:   &layout.Col{Text: " " + opCount(nOps), Style: p.style(colActionOps), Elide: layout.Tail, Order: 3},
		isHeader: true,
	}
}

// railGutter builds a gutter of the plan rail plus a marker in color c.
func (p *planRenderer) railGutter(marker string, c ansi.ANSI) string {
	return p.fmt.fmtMsg(colPlanRail, p.glyphs.planRail) + p.fmt.fmtMsg(c, " "+marker)
}

func (p *planRenderer) verboseLines(act dag.Action, indexWidth int, v signal.Verbosity) []planLine {
	start := p.glyphs.actionStart
	if len(act.Layers) == 0 {
		start = p.glyphs.actionStartNoOp
	}

	lines := []planLine{{
		gutter:   p.railGutter(start, colActionRail),
		id:       p.idStr(act.Index, indexWidth, act.Kind),
		desc:     p.descStr(act.Desc),
		isHeader: true,
	}}

	ops := dag.FlattenLayers(act.Layers)
	children := dag.BuildDepTree(ops)
	roots := dag.FindRoots(ops)
	for i, root := range roots {
		p.collectOpTreeLines(&lines, root, children, []bool{true}, i == len(roots)-1, v)
	}

	lines = append(lines, planLine{
		gutter: p.railGutter(p.glyphs.actionEnd, colActionRail),
	})
	return lines
}

func (p *planRenderer) collectOpTreeLines(
	lines *[]planLine,
	op result.PlannedOp,
	children map[int][]result.PlannedOp,
	prefix []bool,
	isLast bool,
	v signal.Verbosity,
) {
	var b strings.Builder
	b.WriteString(p.fmt.fmtMsg(colPlanRail, p.glyphs.planRail))
	b.WriteString(" ")
	for i, cont := range prefix {
		seg := p.glyphs.actionIndent
		if cont {
			seg = p.glyphs.actionRail + " "
		}
		if i == 0 {
			b.WriteString(p.fmt.fmtMsg(colActionRail, seg))
		} else {
			b.WriteString(p.fmt.fmtMsg(colOpRail, seg))
		}
	}
	conn := p.glyphs.opBranch
	if isLast {
		conn = p.glyphs.opLast
	}
	b.WriteString(p.fmt.fmtMsg(colOpRail, conn))

	ln := planLine{
		gutter: b.String(),
		id:     p.fmt.fmtMsg(colOpHeader, op.DisplayID),
	}
	if v >= signal.VV && op.Template != nil {
		if text, ok := template.Render(*op.Template); ok {
			ln.detail = &layout.Col{Text: " (" + text + ")", Style: p.style(colOpDesc), Elide: layout.Middle, Order: 3}
		}
	}
	*lines = append(*lines, ln)

	kids := children[op.Index]
	for i, child := range kids {
		p.collectOpTreeLines(lines, child, children, append(prefix, !isLast), i == len(kids)-1, v)
	}
}

// measureLayout
// -----------------------------------------------------------------------------

func (p *planRenderer) measureLayout(actions []actionEntry) planLayout {
	var ly planLayout
	ly.depsStrs = make([]string, len(actions))

	for i, ae := range actions {
		ly.depsStrs[i] = p.fmtDeps(ae.deps)
		depsW := layout.VisibleLen(ly.depsStrs[i])
		ly.maxDepsW = max(ly.maxDepsW, depsW)
		if ae.layerSize > 1 {
			ly.hasParallel = true
			ly.maxParDepsW = max(ly.maxParDepsW, depsW)
		}
		for _, ln := range ae.lines {
			core := layout.VisibleLen(ln.gutter) + layout.VisibleLen(ln.id)
			label := core + layout.VisibleLen(ln.desc)
			full := label
			if ln.detail != nil {
				full += layout.VisibleLen(ln.detail.Text)
			}
			ly.maxCoreW = max(ly.maxCoreW, core)
			ly.maxLabelW = max(ly.maxLabelW, label)
			ly.maxContentW = max(ly.maxContentW, full)
		}
	}
	return ly
}

// renderActions
// -----------------------------------------------------------------------------

func (p *planRenderer) renderActions(out []renderEvent, actions []actionEntry, ly *planLayout) []renderEvent {
	bracketGlyphW := layout.VisibleLen(p.glyphs.parallelTop + " " + p.glyphs.parallelLabel)

	// Structure lives in its own block to the right of all content: the deps
	// column, then (for parallel actions) the bracket. rightW is its reserved
	// width measured from the content column.
	rightW := gap + ly.maxDepsW
	if ly.hasParallel {
		rightW = max(rightW, gap+ly.maxParDepsW+gap+bracketGlyphW)
	}
	if ly.maxDepsW == 0 && !ly.hasParallel {
		rightW = 0
	}

	// Content column: as wide as the widest line wants, capped so the structure
	// still fits, but never below the protected core (gutter + id). Op detail
	// elides to buy width; below the label width the action descs drop; below the
	// core width the structure overflows and we warn.
	contentCol := ly.maxContentW
	if p.width > 0 {
		contentCol = max(min(ly.maxContentW, p.width-rightW), ly.maxCoreW)
	}
	showDesc := p.width <= 0 || contentCol >= ly.maxLabelW
	ly.minWidth = ly.maxCoreW + rightW
	ly.tooNarrow = p.width > 0 && p.width < ly.minWidth

	bracketCol := contentCol + gap + ly.maxParDepsW + gap

	for i, ae := range actions {
		lastLineIdx := len(ae.lines) - 1
		parallel := ae.layerSize > 1
		for lineIdx, ln := range ae.lines {
			body, bodyW, _ := layout.Fit(p.lineCols(ln, showDesc), contentCol, 0)

			line, cur := body, bodyW
			if ln.isHeader && ly.depsStrs[i] != "" {
				line += pad(contentCol-cur+gap) + p.fmt.fmtMsg(colPlanDeps, ly.depsStrs[i])
				cur = contentCol + gap + layout.VisibleLen(ly.depsStrs[i])
			}
			if parallel {
				line += pad(bracketCol-cur) + p.fmt.fmtMsg(colPlanBracket, p.bracketSegment(ae, lineIdx, lastLineIdx))
			}
			out = append(out, renderEvent{stream: streamOut, line: line, wrap: true})
		}
	}
	return out
}

// lineCols assembles a line's columns: gutter and id are protected (Fixed); the
// desc is included only when showDesc; the detail (if any) elides.
func (p *planRenderer) lineCols(ln planLine, showDesc bool) []layout.Col {
	cols := []layout.Col{
		{Text: ln.gutter, Elide: layout.Fixed},
		{Text: ln.id, Elide: layout.Fixed},
	}
	if showDesc && ln.desc != "" {
		cols = append(cols, layout.Col{Text: ln.desc, Elide: layout.Fixed})
	}
	if ln.detail != nil {
		cols = append(cols, *ln.detail)
	}
	return cols
}

func pad(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func (p *planRenderer) bracketSegment(ae actionEntry, lineIdx, lastLineIdx int) string {
	switch {
	case ae.posInLayer == 0 && lineIdx == 0:
		return p.glyphs.parallelTop + " " + p.glyphs.parallelLabel
	case ae.posInLayer == ae.layerSize-1 && lineIdx == lastLineIdx:
		return p.glyphs.parallelBot
	default:
		return p.glyphs.parallelMid
	}
}

func (p *planRenderer) narrowWarning(minWidth int) renderEvent {
	msg := fmt.Sprintf("terminal too narrow for the plan structure: need %d columns, have %d", minWidth, p.width)
	line := p.fmt.fmtfMsg(colDiagWarning, "%s %s", glyphR(p.glyphs.warn), msg)
	return renderEvent{stream: streamErr, wrap: true, line: line}
}

// Helpers
// -----------------------------------------------------------------------------

func kindSuffix(kind string) string {
	if kind == "" {
		return ""
	}
	return " " + kind
}

func opCount(n int) string {
	switch n {
	case 0:
		return "(noop)"
	case 1:
		return "(1 op)"
	default:
		return fmt.Sprintf("(%d ops)", n)
	}
}

func (p *planRenderer) fmtDeps(deps []int) string {
	if len(deps) == 0 {
		return ""
	}
	parts := make([]string, len(deps))
	for i, d := range deps {
		parts[i] = strconv.Itoa(d)
	}
	return p.glyphs.depsArrow + " [" + strings.Join(parts, ", ") + "]"
}

func digits10(i int) int {
	if i == 0 {
		return 1
	}
	n := 0
	for i > 0 {
		i /= 10
		n++
	}
	return n
}
