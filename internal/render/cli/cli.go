// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/render/ansi"
	"scampi.dev/scampi/internal/render/layout"
	"scampi.dev/scampi/internal/secret"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/spec"
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
	// Stdout and Stderr default to os.Stdout / os.Stderr when nil. Injectable
	// so tests can capture output; width and TTY detection fall back to
	// non-terminal for a writer that is not an *os.File.
	Stdout io.Writer
	Stderr io.Writer
}

// CLI is the terminal output backend: it implements diagnostic.Output,
// rendering both the live event stream (Emit) and the one-shot command
// results (the Render* methods + Legend).
type CLI struct {
	opts   Options
	sink   *sink
	glyphs glyphSet
	store  *diagnostic.SourceStore

	isTTY bool
	width int

	planRenderer *planRenderer
	formatter    *formatter

	// stepDrift accumulates a step's Change events until its Result
	// arrives, so the whole block renders atomically (header, then railed
	// drift). No lock: the Emitter serializes delivery (see diagnostic.Output).
	stepDrift map[int][]event.Change
}

var _ diagnostic.Output = (*CLI)(nil)

// New creates a new CLI renderer.
func New(opts Options, store *diagnostic.SourceStore) *CLI {
	glyphs := fancyGlyphs
	if opts.ForceASCII {
		glyphs = asciiGlyphs
	}

	out := opts.Stdout
	if out == nil {
		out = os.Stdout
	}
	errw := opts.Stderr
	if errw == nil {
		errw = os.Stderr
	}

	width, isTTY := terminalInfo(out)
	useColor := shouldUseColor(opts.ColorMode, isTTY)
	fmt := newFormatter(glyphs, useColor, store, opts.Redactor)

	return &CLI{
		opts:         opts,
		store:        store,
		sink:         newSink(out, errw),
		glyphs:       glyphs,
		isTTY:        isTTY,
		width:        width,
		planRenderer: newPlanRenderer(glyphs, width, opts.Verbosity, fmt),
		formatter:    fmt,
		stepDrift:    map[int][]event.Change{},
	}
}

// terminalInfo derives display width and TTY-ness from the output writer. A
// COLUMNS override always wins; otherwise only a real terminal (*os.File) has a
// size and is a TTY, so an injected buffer renders unbounded and uncolored.
func terminalInfo(w io.Writer) (width int, isTTY bool) {
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if n, err := strconv.Atoi(cols); err == nil && n > 0 {
			width = n
		}
	}
	f, ok := w.(*os.File)
	if !ok {
		return width, false
	}
	if width == 0 {
		if cw, _, err := term.GetSize(f.Fd()); err == nil && cw > 0 {
			width = cw
		}
	}
	return width, term.IsTerminal(f.Fd())
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
	c.sink.emit(events)
}

// RenderSummary prints the one-line end-of-run summary computed from
// the aggregated ExecutionReport returned by engine.Check / Apply.
func (c *CLI) RenderSummary(rep result.Execution, checkOnly bool) {
	// Count steps, not ops: a step is the unit the user wrote. Ops within a step
	// (ensure content + owner + perm for one copy) are an implementation detail,
	// so "3 would change" for a single copy step would be misleading. Each step
	// counts once, by its overall outcome (failed wins over changed).
	var changed, wouldChange, failed int
	for _, ar := range rep.Steps {
		switch {
		case ar.Summary.Failed+ar.Summary.Aborted > 0:
			failed++
		case checkOnly && ar.Summary.WouldChange > 0:
			wouldChange++
		case !checkOnly && ar.Summary.Changed > 0:
			changed++
		}
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

// RenderPlan prints the per-deploy step plans (one tree each, in
// topo order) and, when the cross-deploy graph has any structure
// worth showing, follows them with a [graph] header section.
func (c *CLI) RenderPlan(plan result.Plan) {
	for _, level := range plan.Levels {
		for _, node := range level.Nodes {
			c.commitRenderEvents(c.planRenderer.renderPlan(node.Detail))
		}
	}
	if plan.HasGraph() {
		c.renderDeployGraph(plan)
	}
}

func (c *CLI) renderDeployGraph(plan result.Plan) {
	f := c.formatter
	var out []renderEvent

	out = append(out, renderEvent{
		stream: streamOut,
		line:   f.fmtMsg(ansi.Magenta().Bold(), "[graph] deploy plan"),
	})
	for _, level := range plan.Levels {
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
		pad := max(kindWidth-layout.VisibleLen(kind), 0)
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
func (c *CLI) RenderInspect(d result.Inspect) {
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
		for l := range strings.SplitSeq(s, "\n") {
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
		glyphPad := max(maxGlyphWidth-layout.VisibleLen(styled), 0)
		line := "  " + styled + strings.Repeat(" ", glyphPad)

		if e.label != "" {
			labelPad := max(maxLabelWidth-len(e.label), 0)
			line += "  " + c.formatter.fmtMsg(ansi.White(), e.label) + strings.Repeat(" ", labelPad)
		}

		if e.desc != "" {
			line += "  " + c.formatter.fmtMsg(ansi.White(), e.desc)
		}
		events = append(events, renderEvent{stream: streamOut, line: line})
	}

	return events
}

func (c *CLI) RenderLegend() {
	var events []renderEvent

	events = append(events, c.legendSection("STATE", []legendEntry{
		{c.glyphs.change, colStepFinishedChanged, "change", "system state was modified"},
		{c.glyphs.ok, colStepFinishedUnchanged, "ok", "already correct, no change needed"},
		{c.glyphs.exec, colOpExecChanged, "exec", "operation executed"},
		{c.glyphs.warn, colOpCheckUnknown, "warn", "non-fatal issue"},
		{c.glyphs.err, colOpExecFailed, "error", "operation failed"},
		{c.glyphs.fatal, colEngineFinishedFatal, "fatal", "unrecoverable failure"},
	})...)
	events = append(events, renderEvent{stream: streamOut, line: ""})

	planBoundary := fmt.Sprintf("%s %s %s", c.glyphs.planStart, c.glyphs.separator, c.glyphs.planEnd)
	stepHeader := fmt.Sprintf("%s [0] copy", c.glyphs.stepStart)
	opBranch := fmt.Sprintf("%s %s CopyCheck", c.glyphs.stepRail, c.glyphs.opBranch)
	opLast := fmt.Sprintf("%s %s CopyExec", c.glyphs.stepRail, c.glyphs.opLast)
	collapsed := fmt.Sprintf("%s  [2] symlink", c.glyphs.stepStartCollapsed)

	events = append(events, c.legendSection("PLAN", []legendEntry{
		{planBoundary, colPlanRail, "", "plan boundary (wraps entire execution)"},
		{c.glyphs.planRail, colPlanRail, "", "plan rail (steps listed inside)"},
		{"", ansi.ANSI{}, "", ""},
		{stepHeader, colStepKind, "", "step start (step with ops)"},
		{opBranch, colOpHeader, "", "op branch"},
		{opLast, colOpHeader, "", "op branch (last)"},
		{c.glyphs.stepEnd, colStepRail, "", "step end"},
		{"", ansi.ANSI{}, "", ""},
		{collapsed, colStepKind, "", "collapsed step (default verbosity)"},
		{"", ansi.ANSI{}, "", ""},
		{c.glyphs.depsArrow + " [N, ...]", colPlanDeps, "", "depends on step N (must complete first)"},
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
			{"cyan", ansi.Cyan(), "", "step context"},
			{"dim", ansi.BrightBlack().Dim(), "", "detail (higher verbosity)"},
		})...)
		events = append(events, renderEvent{stream: streamOut, line: ""})
	}

	c.commitRenderEvents(events)
}

// RenderEvent dispatches by concrete event type to the per-kind renderers.
func (c *CLI) RenderEvent(e event.Event) {
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
	case event.Result:
		c.renderResult(v)
	}
}

func scopeFromCause(c event.Cause) string {
	if c.Kind == event.CauseHook && c.Ref != "" {
		return fmt.Sprintf(" in hook '%s'", c.Ref)
	}
	return ""
}

// renderChange buffers a drift entry; the block renders when the step's Result
// arrives (see renderStepBlock).
func (c *CLI) renderChange(e event.Change) {
	c.stepDrift[e.Step.Index] = append(c.stepDrift[e.Step.Index], e)
}

func (c *CLI) renderProgress(e event.Progress) {
	label := ""
	if e.Total > 0 {
		label = fmt.Sprintf("(%d/%d)", e.Completed, e.Total)
	}
	if e.Current.Kind != "" {
		if label != "" {
			label += " "
		}
		label += stepIDFromRef(e.Current)
	}
	if label == "" {
		return
	}
	c.commitRenderEvents([]renderEvent{{
		stream: streamOut,
		line:   c.formatter.fmtfMsg(colEngineStarted, "[~] %s", label),
	}})
}

// renderResult renders a finished step as one atomic block: a glyph-led header
// (verdict carried by the glyph) followed by its railed drift. The drift was
// buffered from preceding Change events. `ok` steps are hidden below -v.
func (c *CLI) renderResult(e event.Result) {
	drift := c.stepDrift[e.Step.Index]
	delete(c.stepDrift, e.Step.Index)
	// Ops within a step run concurrently, so drift arrives in completion order.
	// Sort by (op id, field) so a step's rows are stable run-to-run, preserving
	// the serial-equivalent-output invariant.
	slices.SortStableFunc(drift, func(a, b event.Change) int {
		if d := cmp.Compare(a.DisplayID, b.DisplayID); d != 0 {
			return d
		}
		return cmp.Compare(a.Drift.Field, b.Drift.Field)
	})
	c.renderStepBlock(e, drift)
}

// deployTag returns a leading lane tag like " [web]" so interleaved deploys are
// followable. Empty for single-lane runs (the engine leaves Name unset), so
// default single-deploy output stays untagged.
func (c *CLI) deployTag(d event.DeployRef) string {
	if d.Name == "" {
		return ""
	}
	return c.formatter.fmtfMsg(colDeployTag, " [%s]", d.Name)
}

func (c *CLI) renderStepBlock(res event.Result, drift []event.Change) {
	v := c.opts.Verbosity

	var glyph string
	var col ansi.ANSI
	switch res.Outcome {
	case event.StepChanged:
		glyph, col = c.glyphs.change, colStepFinishedChanged
	case event.StepFailed:
		glyph, col = c.glyphs.err, colOpExecFailed
	case event.StepUnchanged:
		if v < signal.V {
			return // converged steps are noise at default verbosity
		}
		glyph, col = c.glyphs.ok, colStepFinishedUnchanged
	default:
		return
	}

	header := c.formatter.fmtMsg(col, "  "+glyph) +
		c.deployTag(res.Step.Deploy) +
		c.formatter.fmtfMsg(colStepKind, " [%d]%s", displayIndex(res.Step.Index), kindSuffix(res.Step.Kind)) +
		c.descSuffix(res.Step.Desc) +
		scopeFromCause(res.Cause)
	out := []renderEvent{{stream: streamOut, line: header}}

	// Per-field drift is op-level detail: shown from -v up. At default verbosity
	// the verdict line alone says which steps change, not the field breakdown.
	if v >= signal.V {
		rows := c.driftRows(drift, v)
		for i, r := range rows {
			rail := c.glyphs.opBranch
			if i == len(rows)-1 {
				rail = c.glyphs.opLast
			}
			out = append(out, renderEvent{
				stream: streamOut,
				line:   "      " + c.formatter.fmtMsg(colOpRail, rail) + " " + r,
			})
		}
	}
	c.commitRenderEvents(out)
}

func (c *CLI) descSuffix(desc string) string {
	if desc == "" {
		return ""
	}
	return c.formatter.fmtfMsg(colStepDesc, " %s %s", c.glyphs.stepKindSep, desc)
}

// driftRows formats the visible drift lines for a step, with the op and field
// columns aligned within the block. At -vv each line is prefixed with the op
// that reported it; below that the op identity is elided. Field-less changes
// (signal-only "it changed") carry no row.
func (c *CLI) driftRows(drift []event.Change, v signal.Verbosity) []string {
	type row struct{ opID, field, cur, des string }

	var rs []row
	opW, fieldW := 0, 0
	for _, ch := range drift {
		d := ch.Drift
		if d.Field == "" || d.Verbosity > v {
			continue
		}
		cur := d.Current
		if cur == "" {
			cur = "(absent)"
		}
		r := row{field: d.Field, cur: cur, des: d.Desired}
		if v >= signal.VV {
			r.opID = ch.DisplayID
		}
		opW = max(opW, len(r.opID))
		fieldW = max(fieldW, len(r.field))
		rs = append(rs, r)
	}

	rows := make([]string, len(rs))
	for i, r := range rs {
		var prefix string
		if opW > 0 {
			prefix = c.formatter.fmtfMsg(colOpHeader, "%-*s  ", opW, r.opID)
		}
		rows[i] = prefix + c.formatter.fmtfMsg(colOpDesc, "%-*s  %s %s %s",
			fieldW, r.field, r.cur, c.glyphs.arrow, r.des)
	}
	return rows
}

// displayIndex maps an engine step index (0-based, an array position) to its
// user-facing ordinal (1-based). The single conversion point: engine internals
// stay 0-based, every surface that shows an index to a human — plan, the
// check/apply stream, future --json — routes through here so a "[3]" means the
// same step everywhere. Never print a raw index + 1 anywhere else.
func displayIndex(engineIndex int) int { return engineIndex + 1 }

// stepIDFromRef returns a step display tag built from the StepRef
// fields. Mirrors the formatting used by lifecycle renderers.
func stepIDFromRef(s event.StepRef) string {
	tag := fmt.Sprintf("%d", displayIndex(s.Index))
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
