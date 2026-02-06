package cli

import (
	"fmt"
	"strconv"
	"strings"

	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/render/dag"
	"godoit.dev/doit/render/layout"
	"godoit.dev/doit/render/template"
	"godoit.dev/doit/signal"
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

func (p *planRenderer) renderPlan(e event.PlanEvent) []renderEvent {
	d := *e.Detail

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
	v := p.verbosity

	out = append(out, renderEvent{stream: streamOut, line: ""})

	hdr := p.fmt.fmtMsg(colPlanRail, p.glyphs.planStart)
	hdr += p.fmt.fmtMsg(colPlanHeader, " execution plan")

	if d.UnitID != "" {
		hdr += p.fmt.fmtfMsg(colPlanHeader, ": %s", d.UnitID)
		if d.UnitDesc != "" {
			hdr += p.fmt.fmtfMsg(colPlanHeader, " %s %s", p.glyphs.actionKindSep, d.UnitDesc)
		}
	}

	out = append(out, renderEvent{stream: streamOut, line: hdr})

	planDAG := dag.Build(d)

	digits10 := func(i int) int {
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

	maxIndex := 0
	for _, layer := range planDAG.ActionLayers {
		for _, act := range layer {
			if act.Index > maxIndex {
				maxIndex = act.Index
			}
		}
	}

	indexWidth := digits10(maxIndex)

	type actionEntry struct {
		innerLines []string
		headerIdx  int
		layerSize  int
		posInLayer int
		deps       []int
	}

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

	maxHeaderWidth := 0
	for _, ae := range actions {
		if w := layout.VisibleLen(ae.innerLines[ae.headerIdx]); w > maxHeaderWidth {
			maxHeaderWidth = w
		}
	}

	fmtDeps := func(deps []int) string {
		if len(deps) == 0 {
			return ""
		}
		parts := make([]string, len(deps))
		for i, d := range deps {
			parts[i] = strconv.Itoa(d)
		}
		return "← [" + strings.Join(parts, ", ") + "]"
	}

	depsStrs := make([]string, len(actions))
	maxParDepsWidth := 0

	for i, ae := range actions {
		depsStrs[i] = fmtDeps(ae.deps)
		if ae.layerSize > 1 {
			if w := len(depsStrs[i]); w > maxParDepsWidth {
				maxParDepsWidth = w
			}
		}
	}

	hasParallel := false
	maxParLineWidth := 0
	for _, ae := range actions {
		if ae.layerSize > 1 {
			hasParallel = true
			for _, line := range ae.innerLines {
				if w := layout.VisibleLen(line); w > maxParLineWidth {
					maxParLineWidth = w
				}
			}
		}
	}

	bracketCol := 0
	if hasParallel {
		headerBased := maxHeaderWidth + 2
		if maxParDepsWidth > 0 {
			headerBased = maxHeaderWidth + 2 + maxParDepsWidth + 2
		}
		contentBased := maxParLineWidth + 2
		bracketCol = max(headerBased, contentBased)
	}

	rail := p.fmt.fmtMsg(colPlanRail, p.glyphs.planRail)

	for i, ae := range actions {
		lastLineIdx := len(ae.innerLines) - 1

		for lineIdx, innerLine := range ae.innerLines {
			isHeader := lineIdx == ae.headerIdx
			fullLine := rail + innerLine

			if isHeader {
				if pad := maxHeaderWidth - layout.VisibleLen(innerLine); pad > 0 {
					fullLine += strings.Repeat(" ", pad)
				}
				if depsStrs[i] != "" {
					fullLine += p.fmt.fmtMsg(colPlanDeps, "  "+depsStrs[i])
				}
			}

			if ae.layerSize > 1 {
				var currentWidth int
				if isHeader {
					currentWidth = maxHeaderWidth
					if depsStrs[i] != "" {
						currentWidth += 2 + len(depsStrs[i])
					}
				} else {
					currentWidth = layout.VisibleLen(innerLine)
				}

				pad := bracketCol - currentWidth
				if pad < 1 {
					pad = 1
				}
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

	out = append(out, renderEvent{stream: streamOut, line: p.fmt.fmtMsg(colPlanRail, p.glyphs.planEnd)})
	out = append(out, renderEvent{stream: streamOut, line: ""})

	return out
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
		if text, ok := template.Render(op.Template.ID, op.Template.Text, op.Template.Data); ok {
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

func (p *planRenderer) renderPlanStarted(e event.PlanEvent) []renderEvent {
	d := *e.StartedDetail
	line := p.fmt.fmtfMsg(colPlanStarted, "[plan] planning")
	if d.UnitID != "" {
		line += p.fmt.fmtfMsg(colPlanStartedUnit, " %s", d.UnitID)
	}
	line += p.fmt.fmtMsg(colPlanStarted, " started")
	return []renderEvent{{stream: streamOut, line: line}}
}

func (p *planRenderer) renderPlanFinished(e event.PlanEvent) []renderEvent {
	d := *e.FinishedDetail
	ttlSteps := d.SuccessfulSteps + d.FailedSteps

	if d.FailedSteps > 0 {
		line := p.fmt.fmtfMsg(colPlanFinishedFailed, "[plan]%s planning", glyphR(p.glyphs.fatal))
		if d.UnitID != "" {
			line += p.fmt.fmtfMsg(colPlanFinishedFailedUnit, " %s", d.UnitID)
		}
		line += p.fmt.fmtfMsg(colPlanFinishedFailed,
			" aborted: %d/%d step%s planned, %d/%d step%s failed (%s)",
			d.SuccessfulSteps, ttlSteps, layout.Plural(d.SuccessfulSteps),
			d.FailedSteps, ttlSteps, layout.Plural(d.FailedSteps), d.Duration)
		return []renderEvent{{stream: streamOut, line: line}}
	}

	line := p.fmt.fmtMsg(colPlanFinishedOK, "[plan] planning")
	if d.UnitID != "" {
		line += p.fmt.fmtfMsg(colPlanFinishedOKUnit, " %s", d.UnitID)
	}
	line += p.fmt.fmtfMsg(colPlanFinishedOK,
		" finished: %d step%s planned (%s)",
		d.SuccessfulSteps, layout.Plural(d.SuccessfulSteps), d.Duration)
	return []renderEvent{{stream: streamOut, line: line}}
}

func (p *planRenderer) renderStepPlanned(e event.PlanEvent) []renderEvent {
	return []renderEvent{{
		stream: streamOut,
		line: p.fmt.fmtfMsg(colPlanStepPlanned, "[plan.step]%s #%d %s '%s'",
			glyphR(p.glyphs.ok), e.Step.StepIndex, e.Step.StepKind, e.Step.StepDesc),
	}}
}
