// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/model"
	"scampi.dev/scampi/render/ansi"
	"scampi.dev/scampi/render/layout"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

// Options configures the CLI renderer.
type Options struct {
	ColorMode  signal.ColorMode
	Verbosity  signal.Verbosity
	ForceASCII bool
	// Redactor, if non-nil, is consulted at render time to mask
	// secret values registered during eval (typically via
	// secrets.get). See #281.
	Redactor *secret.Redactor
}

// CLI is the terminal renderer. It implements diagnostic.Displayer
// for streaming events; the public Render* methods serve command
// outputs that the engine returns directly (Plan, Inspect, Index).
type CLI struct {
	opts   Options
	render *renderer
	glyphs glyphSet
	store  *diagnostic.SourceStore

	isTTY bool
	width int

	planRenderer *planRenderer
	formatter    *formatter
}

// New creates a new CLI renderer.
func New(opts Options, store *diagnostic.SourceStore) *CLI {
	glyphs := fancyGlyphs
	if opts.ForceASCII {
		glyphs = asciiGlyphs
	}

	width := func() int {
		if cols := os.Getenv("COLUMNS"); cols != "" {
			if n, err := strconv.Atoi(cols); err == nil && n > 0 {
				return n
			}
		}
		if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
			return w
		}
		return 0
	}()

	isTTY := term.IsTerminal(os.Stdout.Fd())
	useColor := shouldUseColor(opts.ColorMode, isTTY)
	fmt := newFormatter(glyphs, useColor, store, opts.Redactor)

	return &CLI{
		opts:         opts,
		store:        store,
		render:       newRenderer(os.Stdout, os.Stderr, isTTY, glyphs, fmt),
		glyphs:       glyphs,
		isTTY:        isTTY,
		width:        width,
		planRenderer: newPlanRenderer(glyphs, width, opts.Verbosity, fmt),
		formatter:    fmt,
	}
}

func (c *CLI) commitRenderEvents(events []renderEvent) {
	for i := range events {
		// Redact at the central choke point so every line — diagnostic,
		// inspect, plan render, status update — passes through the
		// secret mask. Targeted redaction inside fmtTemplate /
		// EmitInspect is intentional defense-in-depth; this catches
		// anything that builds a renderEvent directly. See #281.
		events[i].line = c.formatter.redact(events[i].line)
		events[i].line = strings.NewReplacer("\n", " ", "\r", "").Replace(events[i].line)
		if !events[i].wrap {
			events[i].line = layout.FitLine(events[i].line, c.width, c.glyphs.ellipsis)
		}
		if c.shouldUseColor() {
			events[i].line += ansi.Reset
		}
	}
	c.render.emitEvents(events)
}

// RenderSummary prints the one-line end-of-run summary computed from
// the aggregated ExecutionReport returned by engine.Check / Apply.
func (c *CLI) RenderSummary(rep model.ExecutionReport, checkOnly bool) {
	var changed, wouldChange, failed int
	for _, ar := range rep.Actions {
		changed += ar.Summary.Changed
		wouldChange += ar.Summary.WouldChange
		failed += ar.Summary.Failed + ar.Summary.Aborted
	}
	var parts []string
	if checkOnly {
		parts = append(parts, fmt.Sprintf("%d would change", wouldChange))
	} else {
		parts = append(parts, fmt.Sprintf("%d changed", changed))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	line := "done: " + strings.Join(parts, ", ")
	col := ansi.Green().Dim()
	if failed > 0 {
		col = ansi.Red().Bold()
	} else if changed > 0 || wouldChange > 0 {
		col = ansi.Yellow()
	}
	c.commitRenderEvents([]renderEvent{{
		stream: streamOut,
		line:   c.formatter.fmtMsg(col, line),
	}})
}

// RenderPlan prints the per-deploy action plans (one tree each, in
// topo order) and, when the cross-deploy graph has any structure
// worth showing, follows them with a [graph] header section.
func (c *CLI) RenderPlan(result engine.PlanResult) {
	for _, level := range result.Levels {
		for _, node := range level.Nodes {
			c.commitRenderEvents(c.planRenderer.renderPlan(node.Detail))
		}
	}
	if result.HasGraph() {
		c.renderDeployGraph(result)
	}
}

func (c *CLI) renderDeployGraph(result engine.PlanResult) {
	f := c.formatter
	var out []renderEvent

	out = append(out, renderEvent{
		stream: streamOut,
		line:   f.fmtMsg(ansi.Magenta().Bold(), "[graph] deploy plan"),
	})
	for _, level := range result.Levels {
		for _, n := range level.Nodes {
			indent := strings.Repeat("  ", level.Index)
			label := f.fmtMsg(ansi.Cyan().Bold(), n.DeployName)
			suffix := ""
			if len(n.After) > 0 {
				suffix = " (after: " + strings.Join(n.After, ", ")
				if len(n.Needs) > 0 {
					suffix += "; needs: " + strings.Join(n.Needs, ", ")
				}
				suffix += ")"
				suffix = f.fmtMsg(ansi.BrightBlack(), suffix)
			}
			out = append(out, renderEvent{
				stream: streamOut,
				line:   "[graph] " + indent + f.glyphs.bullet + " " + label + suffix,
			})
		}
	}
	out = append(out, renderEvent{stream: streamOut, line: ""})
	c.commitRenderEvents(out)
}

// RenderIndexAll prints the step catalog (`scampi index`). Docs are
// expected to be sorted by Kind ascending by the caller.
func (c *CLI) RenderIndexAll(docs []spec.StepDoc) {
	kindWidth := 0
	for _, doc := range docs {
		if w := layout.VisibleLen(doc.Kind); w > kindWidth {
			kindWidth = w
		}
	}

	var events []renderEvent

	events = append(events, renderEvent{
		line:   c.formatter.fmtMsg(ansi.BrightBlack(), "AVAILABLE STEPS"),
		stream: streamOut,
	})
	events = append(events, renderEvent{line: "", stream: streamOut})

	for _, doc := range docs {
		kind := c.formatter.fmtMsg(ansi.BrightCyan().Bold(), doc.Kind)
		pad := kindWidth - layout.VisibleLen(kind)
		if pad < 0 {
			pad = 0
		}
		prefix := "  " + kind + strings.Repeat(" ", pad) + "  "
		indent := 2 + kindWidth + 2

		descWidth := c.width - indent
		if descWidth <= 10 || c.width <= 0 {
			line := prefix + c.formatter.fmtMsg(ansi.White(), doc.Summary)
			events = append(events, renderEvent{line: line, stream: streamOut, wrap: true})
		} else {
			for i, dl := range layout.WrapText(doc.Summary, descWidth) {
				var line string
				if i == 0 {
					line = prefix + c.formatter.fmtMsg(ansi.White(), dl)
				} else {
					line = strings.Repeat(" ", indent) + c.formatter.fmtMsg(ansi.White(), dl)
				}
				events = append(events, renderEvent{line: line, stream: streamOut, wrap: true})
			}
		}
	}

	events = append(events, renderEvent{line: "", stream: streamOut})
	events = append(events, renderEvent{
		line:   c.formatter.fmtMsg(ansi.BrightBlack(), "Use 'scampi index <step>' for details."),
		stream: streamOut,
	})

	c.commitRenderEvents(events)
}

// RenderInspect prints inspect output for one deploy.
func (c *CLI) RenderInspect(d event.InspectDetail) {
	f := c.formatter
	var out []renderEvent

	header := f.fmtfMsg(
		ansi.Blue().Bold(),
		"deploy %q "+f.glyphs.arrow+" %s",
		d.DeployName,
		d.TargetName,
	)
	out = append(out,
		renderEvent{stream: streamOut, line: header},
		renderEvent{stream: streamOut, line: ""},
	)

	for _, entry := range d.Entries {
		line := "  " + f.fmtMsg(ansi.Cyan().Bold(), entry.Kind)
		if entry.Desc != "" {
			line += " " + f.fmtMsg(ansi.Cyan(), entry.Desc)
		}
		out = append(out, renderEvent{stream: streamOut, line: line})

		for _, field := range entry.Fields {
			label := f.fmtfMsg(ansi.BrightBlack(), "%-12s", field.Label)
			out = append(out, renderEvent{
				stream: streamOut,
				line:   "      " + label + " " + f.redact(field.Value),
			})
		}
		out = append(out, renderEvent{stream: streamOut, line: ""})
	}

	c.commitRenderEvents(out)
}

// RenderIndexStep prints the detailed documentation for a single
// step (`scampi index <kind>`).
func (c *CLI) RenderIndexStep(doc spec.StepDoc) {
	var events []renderEvent

	events = append(events, renderEvent{
		line:   c.formatter.fmtMsg(ansi.BrightCyan().Bold(), strings.ToUpper(doc.Kind)),
		stream: streamOut,
	})

	if doc.Summary != "" {
		events = append(events, renderEvent{line: "", stream: streamOut})
		summaryWidth := c.width - 2
		if summaryWidth <= 10 || c.width <= 0 {
			events = append(events, renderEvent{
				line: c.formatter.fmtMsg(ansi.White(), "  "+doc.Summary), stream: streamOut, wrap: true,
			})
		} else {
			for _, sl := range layout.WrapText(doc.Summary, summaryWidth) {
				events = append(events, renderEvent{
					line: c.formatter.fmtMsg(ansi.White(), "  "+sl), stream: streamOut, wrap: true,
				})
			}
		}
	}

	if len(doc.Fields) > 0 {
		events = append(events, renderEvent{line: "", stream: streamOut})
		events = append(events, renderEvent{
			line:   c.formatter.fmtMsg(ansi.BrightBlack(), "FIELDS"),
			stream: streamOut,
		})

		exclusiveGroups, required, optional := partitionFields(doc.Fields)

		for _, g := range exclusiveGroups {
			events = append(events, renderEvent{line: "", stream: streamOut})
			events = append(events, renderEvent{
				line:   "  " + c.formatter.fmtMsg(ansi.BrightBlack(), "Provide exactly one of:"),
				stream: streamOut,
			})
			events = append(events, renderEvent{line: "", stream: streamOut})
			events = append(events, c.renderFieldRows(g)...)
		}

		if len(required) > 0 {
			events = append(events, renderEvent{line: "", stream: streamOut})
			events = append(events, renderEvent{
				line:   "  " + c.formatter.fmtMsg(ansi.BrightBlack(), "Required:"),
				stream: streamOut,
			})
			events = append(events, renderEvent{line: "", stream: streamOut})
			events = append(events, c.renderFieldRows(required)...)
		}

		if len(optional) > 0 {
			events = append(events, renderEvent{line: "", stream: streamOut})
			events = append(events, renderEvent{
				line:   "  " + c.formatter.fmtMsg(ansi.BrightBlack(), "Optional:"),
				stream: streamOut,
			})
			events = append(events, renderEvent{line: "", stream: streamOut})
			events = append(events, c.renderFieldRows(optional)...)
		}
	}

	// Per-field examples and snippet, gated by verbosity.
	hasFieldExamples := false
	for _, f := range doc.Fields {
		if len(f.Examples) > 0 {
			hasFieldExamples = true
			break
		}
	}

	snippets := doc.Examples()
	v := c.opts.Verbosity

	switch {
	case v >= signal.VV:
		if hasFieldExamples {
			events = append(events, c.renderFieldExamples(doc.Fields)...)
		}
		events = append(events, c.renderSnippets(snippets)...)
	case v >= signal.V:
		if hasFieldExamples {
			events = append(events, c.renderFieldExamples(doc.Fields)...)
		}
		events = append(events, renderEvent{line: "", stream: streamOut})
		events = append(events, renderEvent{
			line: c.formatter.fmtMsg(ansi.BrightBlack(),
				fmt.Sprintf("For a copy-pasteable snippet, run 'scampi index %s -vv'.", doc.Kind)),
			stream: streamOut,
		})
	default:
		if hasFieldExamples || len(snippets) > 0 {
			events = append(events, renderEvent{line: "", stream: streamOut})
			events = append(events, renderEvent{
				line: c.formatter.fmtMsg(ansi.BrightBlack(),
					fmt.Sprintf("For examples, run 'scampi index %s -v'.", doc.Kind)),
				stream: streamOut,
			})
		}
	}

	c.commitRenderEvents(events)
}

func fieldDescWithDefault(f spec.FieldDoc) string {
	if f.Default == "" {
		return f.Desc
	}
	return fmt.Sprintf("%s (default: %s)", f.Desc, f.Default)
}

// partitionFields splits fields into exclusive groups (in encounter order),
// required fields, and optional fields.
func partitionFields(fields []spec.FieldDoc) (groups [][]spec.FieldDoc, required, optional []spec.FieldDoc) {
	seen := make(map[string]int) // group name → index into groups
	for _, f := range fields {
		switch {
		case f.Exclusive != "":
			idx, ok := seen[f.Exclusive]
			if !ok {
				idx = len(groups)
				seen[f.Exclusive] = idx
				groups = append(groups, nil)
			}
			groups[idx] = append(groups[idx], f)
		case f.Required:
			required = append(required, f)
		default:
			optional = append(optional, f)
		}
	}
	return groups, required, optional
}

func (c *CLI) renderFieldRows(fields []spec.FieldDoc) []renderEvent {
	nameW, typeW := 0, 0
	for _, f := range fields {
		if len(f.Name) > nameW {
			nameW = len(f.Name)
		}
		if len(f.Type) > typeW {
			typeW = len(f.Type)
		}
	}

	indent := 2 + nameW + 3 + typeW + 3

	var events []renderEvent
	for _, f := range fields {
		prefix := "  " +
			c.formatter.fmtMsg(ansi.Cyan(), fmt.Sprintf("%-*s", nameW, f.Name)) +
			"   " +
			c.formatter.fmtMsg(ansi.BrightBlack(), fmt.Sprintf("%-*s", typeW, f.Type)) +
			"   "

		desc := fieldDescWithDefault(f)
		descWidth := c.width - indent
		if descWidth <= 10 || c.width <= 0 {
			events = append(events, renderEvent{
				line: prefix + c.formatter.fmtMsg(ansi.White(), desc), stream: streamOut, wrap: true,
			})
		} else {
			for i, dl := range layout.WrapText(desc, descWidth) {
				var line string
				if i == 0 {
					line = prefix + c.formatter.fmtMsg(ansi.White(), dl)
				} else {
					line = strings.Repeat(" ", indent) + c.formatter.fmtMsg(ansi.White(), dl)
				}
				events = append(events, renderEvent{line: line, stream: streamOut, wrap: true})
			}
		}
	}
	return events
}

func (c *CLI) renderFieldExamples(fields []spec.FieldDoc) []renderEvent {
	var events []renderEvent
	events = append(events, renderEvent{line: "", stream: streamOut})
	events = append(events, renderEvent{
		line:   c.formatter.fmtMsg(ansi.BrightBlack(), "EXAMPLES"),
		stream: streamOut,
	})
	events = append(events, renderEvent{line: "", stream: streamOut})

	nameW := 0
	for _, f := range fields {
		if len(f.Examples) > 0 && len(f.Name) > nameW {
			nameW = len(f.Name)
		}
	}

	for _, f := range fields {
		if len(f.Examples) == 0 {
			continue
		}
		line := "  " +
			c.formatter.fmtMsg(ansi.Cyan(), fmt.Sprintf("%-*s", nameW, f.Name)) +
			"   " +
			c.formatter.fmtMsg(ansi.BrightBlack(), strings.Join(f.Examples, "   "))
		events = append(events, renderEvent{
			line:   line,
			stream: streamOut,
		})
	}
	return events
}

func (c *CLI) renderSnippets(snippets []string) []renderEvent {
	var events []renderEvent
	events = append(events, renderEvent{line: "", stream: streamOut})
	events = append(events, renderEvent{
		line:   c.formatter.fmtMsg(ansi.BrightBlack(), "EXAMPLE SNIPPET"),
		stream: streamOut,
	})
	for i, s := range snippets {
		if i > 0 {
			events = append(events, renderEvent{line: "", stream: streamOut})
		}
		events = append(events, renderEvent{line: "", stream: streamOut})
		for _, l := range strings.Split(s, "\n") {
			events = append(events, renderEvent{
				line:   c.formatter.fmtMsg(ansi.BrightBlack().Dim(), "  "+l),
				stream: streamOut,
			})
		}
	}
	return events
}

type legendEntry struct {
	glyph string
	color ansi.ANSI
	label string
	desc  string
}

func (c *CLI) legendSection(header string, entries []legendEntry) []renderEvent {
	var events []renderEvent

	events = append(events, renderEvent{
		stream: streamOut,
		line:   c.formatter.fmtMsg(ansi.BrightBlack(), header),
	})
	events = append(events, renderEvent{stream: streamOut, line: ""})

	maxGlyphWidth := 0
	maxLabelWidth := 0
	for _, e := range entries {
		if w := layout.VisibleLen(c.formatter.fmtMsg(e.color, e.glyph)); w > maxGlyphWidth {
			maxGlyphWidth = w
		}
		if w := len(e.label); w > maxLabelWidth {
			maxLabelWidth = w
		}
	}

	for _, e := range entries {
		if e.glyph == "" && e.desc == "" {
			events = append(events, renderEvent{stream: streamOut, line: ""})
			continue
		}

		styled := c.formatter.fmtMsg(e.color, e.glyph)
		glyphPad := maxGlyphWidth - layout.VisibleLen(styled)
		if glyphPad < 0 {
			glyphPad = 0
		}
		line := "  " + styled + strings.Repeat(" ", glyphPad)

		if e.label != "" {
			labelPad := maxLabelWidth - len(e.label)
			if labelPad < 0 {
				labelPad = 0
			}
			line += "  " + c.formatter.fmtMsg(ansi.White(), e.label) + strings.Repeat(" ", labelPad)
		}

		if e.desc != "" {
			line += "  " + c.formatter.fmtMsg(ansi.White(), e.desc)
		}
		events = append(events, renderEvent{stream: streamOut, line: line})
	}

	return events
}

func (c *CLI) EmitLegend() {
	var events []renderEvent

	events = append(events, c.legendSection("STATE", []legendEntry{
		{c.glyphs.change, colActionFinishedChanged, "change", "system state was modified"},
		{c.glyphs.ok, colActionFinishedUnchanged, "ok", "already correct, no change needed"},
		{c.glyphs.exec, colOpExecChanged, "exec", "operation executed"},
		{c.glyphs.warn, colOpCheckUnknown, "warn", "non-fatal issue"},
		{c.glyphs.err, colOpExecFailed, "error", "operation failed"},
		{c.glyphs.fatal, colEngineFinishedFatal, "fatal", "unrecoverable failure"},
	})...)
	events = append(events, renderEvent{stream: streamOut, line: ""})

	planBoundary := fmt.Sprintf("%s %s %s", c.glyphs.planStart, c.glyphs.separator, c.glyphs.planEnd)
	actionHeader := fmt.Sprintf("%s [0] copy", c.glyphs.actionStart)
	opBranch := fmt.Sprintf("%s %s CopyCheck", c.glyphs.actionRail, c.glyphs.opBranch)
	opLast := fmt.Sprintf("%s %s CopyExec", c.glyphs.actionRail, c.glyphs.opLast)
	collapsed := fmt.Sprintf("%s  [2] symlink", c.glyphs.actionStartCollapsed)

	events = append(events, c.legendSection("PLAN", []legendEntry{
		{planBoundary, colPlanRail, "", "plan boundary (wraps entire execution)"},
		{c.glyphs.planRail, colPlanRail, "", "plan rail (actions listed inside)"},
		{"", ansi.ANSI{}, "", ""},
		{actionHeader, colActionKind, "", "action start (step with ops)"},
		{opBranch, colOpHeader, "", "op branch"},
		{opLast, colOpHeader, "", "op branch (last)"},
		{c.glyphs.actionEnd, colActionRail, "", "action end"},
		{"", ansi.ANSI{}, "", ""},
		{collapsed, colActionKind, "", "collapsed action (default verbosity)"},
		{"", ansi.ANSI{}, "", ""},
		{c.glyphs.depsArrow + " [N, ...]", colPlanDeps, "", "depends on action N (must complete first)"},
		{c.glyphs.parallelTop, colPlanBracket, "", "parallel execution group"},
		{c.glyphs.parallelMid, colPlanBracket, "", ""},
		{c.glyphs.parallelBot, colPlanBracket, "", ""},
		{c.glyphs.parallelLabel, colPlanBracket, "", "group boundary (engine waits for all)"},
	})...)
	events = append(events, renderEvent{stream: streamOut, line: ""})

	if c.shouldUseColor() {
		events = append(events, c.legendSection("COLORS", []legendEntry{
			{"yellow", ansi.Yellow(), "", "mutation, system state changed"},
			{"green", ansi.Green(), "", "correct, no change needed"},
			{"red", ansi.Red(), "", "failure"},
			{"blue", ansi.Blue(), "", "engine and plan boundaries"},
			{"magenta", ansi.Magenta(), "", "plan structure"},
			{"cyan", ansi.Cyan(), "", "action context"},
			{"dim", ansi.BrightBlack().Dim(), "", "detail (higher verbosity)"},
		})...)
		events = append(events, renderEvent{stream: streamOut, line: ""})
	}

	c.commitRenderEvents(events)
}

func (c *CLI) Interrupt() {
	c.render.interrupted.Store(true)
}

func (c *CLI) Close() {
	c.render.close()
}

// Raise emits the event produced by err.
func (c *CLI) Raise(err diagnostic.Raisable) {
	c.Emit(err.Diagnostic())
}

// Emit dispatches by concrete event type to the per-kind renderers.
func (c *CLI) Emit(e event.Event) {
	switch v := e.(type) {
	case event.Error:
		c.renderDiagnostic(signal.Error, scopeFromCause(v.Cause), v.Template)
	case event.Warning:
		c.renderDiagnostic(signal.Warning, scopeFromCause(v.Cause), v.Template)
	case event.Info:
		c.renderDiagnostic(signal.Info, scopeFromCause(v.Cause), v.Template)
	case event.Change:
		c.renderChange(v)
	case event.Progress:
		c.renderProgress(v)
	}
}

func scopeFromCause(c event.Cause) string {
	if c.Kind == event.CauseHook && c.Ref != "" {
		return fmt.Sprintf(" in hook '%s'", c.Ref)
	}
	return ""
}

func (c *CLI) renderChange(e event.Change) {
	var verb string
	var col ansi.ANSI
	switch e.Phase {
	case event.ChangePlanned:
		verb = "would change"
		col = colOpCheckUnsatisfied
	case event.ChangeExecuted:
		verb = "changed"
		col = colOpExecChanged
	default:
		return
	}
	stepID := stepIDFromRef(e.Step)
	prefix := fmt.Sprintf("[%s]", stepID)
	if e.DisplayID != "" {
		prefix = fmt.Sprintf("[%s] '%s'", stepID, e.DisplayID)
	}
	if e.Drift.Field == "" {
		c.commitRenderEvents([]renderEvent{{
			stream: streamOut,
			line:   c.formatter.fmtfMsg(col, "%s%s %s", prefix, glyphR(c.glyphs.exec), verb),
		}})
		return
	}
	c.commitRenderEvents([]renderEvent{{
		stream: streamOut,
		line: c.formatter.fmtfMsg(col, "%s %s %s: %s -> %s",
			prefix, verb, e.Drift.Field, e.Drift.Current, e.Drift.Desired),
	}})
}

func (c *CLI) renderProgress(e event.Progress) {
	c.commitRenderEvents([]renderEvent{{
		stream: streamOut,
		line:   c.formatter.fmtfMsg(colEngineStarted, "[~] %s", e.Text),
	}})
}

// stepIDFromRef returns a step display tag built from the StepRef
// fields. Mirrors the formatting used by lifecycle renderers.
func stepIDFromRef(s event.StepRef) string {
	tag := fmt.Sprintf("%d", s.Index)
	if s.Kind != "" {
		tag += "|" + s.Kind
	}
	return tag
}

func (c *CLI) renderDiagnostic(sev signal.Severity, scope string, tmpl event.Template) {
	var glyph, suffix string
	var col ansi.ANSI

	switch sev {
	case signal.Info:
		glyph = c.glyphs.hint
		col = colDiagInfo
		suffix = "info"
	case signal.Warning:
		glyph = c.glyphs.warn
		col = colDiagWarning
		suffix = "warning"
	case signal.Error:
		glyph = c.glyphs.err
		col = colDiagError
		suffix = "error"
	default:
		panic(errs.BUG("unhandled diagnostic severity %d", sev))
	}

	label := "." + suffix

	var events []renderEvent
	for _, l := range c.formatter.fmtTemplate(tmpl, label, scope, glyph, col, colDiagHelp) {
		events = append(events, renderEvent{stream: streamErr, line: l, wrap: true})
	}
	c.commitRenderEvents(events)
}

func (c *CLI) shouldUseColor() bool {
	return shouldUseColor(c.opts.ColorMode, c.isTTY)
}

func shouldUseColor(mode signal.ColorMode, isTTY bool) bool {
	switch mode {
	case signal.ColorAlways:
		return true
	case signal.ColorNever:
		return false
	case signal.ColorAuto:
		return isTTY
	default:
		return false
	}
}
