// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/render/dag"
	"scampi.dev/scampi/render/layout"
	"scampi.dev/scampi/render/template"
	"scampi.dev/scampi/signal"
)

const minWidePlanCols = 70

type planRenderer struct {
	glyphs    glyphSet
	width     int
	verbosity signal.Verbosity
	fmt       *formatter
}

func newPlanRenderer(glyphs glyphSet, width int, verbosity signal.Verbosity, f *formatter) *planRenderer {
	return &planRenderer{glyphs: glyphs, width: width, verbosity: verbosity, fmt: f}
}

type actionEntry struct {
	innerLines []string
	headerIdx  int
	layerSize  int
	posInLayer int
	deps       []int
}

type planLayout struct {
	maxHeaderWidth  int
	depsStrs        []string
	maxParDepsWidth int
	hasParallel     bool
	bracketCol      int
}

func (p *planRenderer) renderPlan(d event.PlanDetail) []renderEvent {
	if p.width < minWidePlanCols {
		for i := range d.Actions {
			for j := range d.Actions[i].Ops {
				if tmpl := d.Actions[i].Ops[j].Template; tmpl != nil {
					tmpl.Text = ""
				}
			}
		}
	}

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
	out = p.renderActions(out, actions, ly)

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
			ae := actionEntry{
				layerSize:  len(layer),
				posInLayer: posInLayer,
				deps:       act.DependsOn,
				headerIdx:  0,
			}

			kind := ""
			if act.Kind != "" {
				kind = fmt.Sprintf(" %s", act.Kind)
			}
			desc := ""
			if act.Desc != "" {
				desc = fmt.Sprintf(" %s %s", p.glyphs.actionKindSep, act.Desc)
			}

			if v == signal.Quiet {
				nOps := 0
				for _, l := range act.Layers {
					nOps += len(l)
				}

				line := p.fmt.fmtMsg(colActionKind, " "+p.glyphs.actionStartCollapsed) +
					p.fmt.fmtfMsg(colActionKind, " [%*d]%s", indexWidth, act.Index, kind)
				if desc != "" {
					line += p.fmt.fmtMsg(colActionDesc, desc)
				}

				var opLine string
				switch nOps {
				case 0:
					opLine = " (noop)"
				case 1:
					opLine = " (1 op)"
				default:
					opLine = fmt.Sprintf(" (%d ops)", nOps)
				}
				line += p.fmt.fmtMsg(colActionOps, opLine)

				ae.innerLines = []string{line}
			} else {
				gutter := p.glyphs.actionStart
				if len(act.Layers) == 0 {
					gutter = p.glyphs.actionStartNoOp
				}

				headerLine := " " + p.fmt.fmtMsg(colActionRail, gutter) +
					p.fmt.fmtfMsg(colActionKind, " [%*d]%s", indexWidth, act.Index, kind)
				if desc != "" {
					headerLine += p.fmt.fmtMsg(colActionDesc, desc)
				}

				ae.innerLines = []string{headerLine}

				ops := dag.FlattenLayers(act.Layers)
				children := dag.BuildDepTree(ops)
				roots := dag.FindRoots(ops)

				for i, root := range roots {
					p.collectOpTreeLines(&ae.innerLines, root, children, []bool{true}, i == len(roots)-1, v)
				}

				ae.innerLines = append(ae.innerLines, " "+p.fmt.fmtMsg(colActionRail, p.glyphs.actionEnd))
			}

			actions = append(actions, ae)
		}
	}

	return actions
}

// measureLayout
// -----------------------------------------------------------------------------

func (p *planRenderer) measureLayout(actions []actionEntry) planLayout {
	var ly planLayout

	for _, ae := range actions {
		if w := layout.VisibleLen(ae.innerLines[ae.headerIdx]); w > ly.maxHeaderWidth {
			ly.maxHeaderWidth = w
		}
	}

	ly.depsStrs = make([]string, len(actions))

	for i, ae := range actions {
		ly.depsStrs[i] = p.fmtDeps(ae.deps)
		if ae.layerSize > 1 {
			if w := layout.VisibleLen(ly.depsStrs[i]); w > ly.maxParDepsWidth {
				ly.maxParDepsWidth = w
			}
		}
	}

	maxParLineWidth := 0
	for _, ae := range actions {
		if ae.layerSize > 1 {
			ly.hasParallel = true
			for _, line := range ae.innerLines {
				if w := layout.VisibleLen(line); w > maxParLineWidth {
					maxParLineWidth = w
				}
			}
		}
	}

	if ly.hasParallel {
		headerBased := ly.maxHeaderWidth + 2
		if ly.maxParDepsWidth > 0 {
			headerBased = ly.maxHeaderWidth + 2 + ly.maxParDepsWidth + 2
		}
		contentBased := maxParLineWidth + 2
		ly.bracketCol = max(headerBased, contentBased)
	}

	return ly
}

// renderActions
// -----------------------------------------------------------------------------

func (p *planRenderer) renderActions(
	out []renderEvent,
	actions []actionEntry,
	ly planLayout,
) []renderEvent {
	rail := p.fmt.fmtMsg(colPlanRail, p.glyphs.planRail)

	for i, ae := range actions {
		lastLineIdx := len(ae.innerLines) - 1

		for lineIdx, innerLine := range ae.innerLines {
			isHeader := lineIdx == ae.headerIdx
			fullLine := rail + innerLine

			if isHeader {
				if pad := ly.maxHeaderWidth - layout.VisibleLen(innerLine); pad > 0 {
					fullLine += strings.Repeat(" ", pad)
				}
				if ly.depsStrs[i] != "" {
					fullLine += p.fmt.fmtMsg(colPlanDeps, "  "+ly.depsStrs[i])
				}
			}

			if ae.layerSize > 1 {
				var currentWidth int
				if isHeader {
					currentWidth = ly.maxHeaderWidth
					if ly.depsStrs[i] != "" {
						currentWidth += 2 + layout.VisibleLen(ly.depsStrs[i])
					}
				} else {
					currentWidth = layout.VisibleLen(innerLine)
				}

				pad := max(ly.bracketCol-currentWidth, 1)
				fullLine += strings.Repeat(" ", pad)

				switch {
				case ae.posInLayer == 0 && isHeader:
					fullLine += p.fmt.fmtMsg(colPlanBracket, p.glyphs.parallelTop+" "+p.glyphs.parallelLabel)
				case ae.posInLayer == ae.layerSize-1 && lineIdx == lastLineIdx:
					fullLine += p.fmt.fmtMsg(colPlanBracket, p.glyphs.parallelBot)
				default:
					fullLine += p.fmt.fmtMsg(colPlanBracket, p.glyphs.parallelMid)
				}
			}

			out = append(out, renderEvent{stream: streamOut, line: fullLine})
		}
	}

	return out
}

// Helpers
// -----------------------------------------------------------------------------

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

func (p *planRenderer) collectOpTreeLines(
	lines *[]string,
	op event.PlannedOp,
	children map[int][]event.PlannedOp,
	prefix []bool,
	isLast bool,
	v signal.Verbosity,
) {
	var b strings.Builder
	b.WriteString(" ")

	for i, cont := range prefix {
		var seg string
		if cont {
			seg = p.glyphs.actionRail + " "
		} else {
			seg = p.glyphs.actionIndent
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

	line := b.String()
	line += p.fmt.fmtMsg(colOpHeader, op.DisplayID)

	if v >= signal.VV && op.Template != nil {
		if text, ok := template.Render(*op.Template); ok {
			line += p.fmt.fmtfMsg(colOpDesc, " (%s)", text)
		}
	}

	*lines = append(*lines, line)

	kids := children[op.Index]
	for i, child := range kids {
		last := i == len(kids)-1
		p.collectOpTreeLines(lines, child, children, append(prefix, !isLast), last, v)
	}
}
