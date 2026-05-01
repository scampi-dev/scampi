// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/term"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/model"
	"scampi.dev/scampi/render/ansi"
	"scampi.dev/scampi/render/layout"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

// Options configures the CLI renderer.
type Options struct {
	ColorMode  signal.ColorMode
	Verbosity  signal.Verbosity
	ForceASCII bool
}

type cli struct {
	opts    Options
	render  *renderer
	glyphs  glyphSet
	store   *diagnostic.SourceStore
	actions sync.Map // map[string]*actionState

	isTTY bool
	width int

	planRenderer *planRenderer
	formatter    *formatter
}

type actionState struct {
	id string
}

// New creates a new CLI renderer.
func New(opts Options, store *diagnostic.SourceStore) diagnostic.Displayer {
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
	fmt := newFormatter(glyphs, useColor, store)
	plan := newPlanRenderer(glyphs, width, opts.Verbosity, fmt)

	return &cli{
		opts:         opts,
		store:        store,
		render:       newRenderer(os.Stdout, os.Stderr, isTTY, glyphs, fmt),
		glyphs:       glyphs,
		isTTY:        isTTY,
		width:        width,
		planRenderer: plan,
		formatter:    fmt,
	}
}

func (c *cli) shouldRender(chatty event.Chattiness) bool {
	v := c.opts.Verbosity
	switch chatty {
	case event.Yappy:
		return v >= signal.VVV
	case event.Chatty:
		return v >= signal.VV
	case event.Normal:
		return v >= signal.V
	case event.Reserved, event.Subtle:
		return true
	default:
		return true
	}
}

func (c *cli) commitRenderEvents(events []renderEvent) {
	for i := range events {
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

func (c *cli) EmitEngineLifecycle(e event.EngineEvent) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	switch e.Kind {
	case event.EngineStarted:
		c.commitRenderEvents(c.renderEngineStarted(e))
	case event.EngineConnecting:
		c.commitRenderEvents(c.renderEngineConnecting(e))
	case event.EngineFinished:
		c.commitRenderEvents(c.renderEngineFinished(e))
	default:
		panic(errs.BUG("unknown engine event kind %q (0x%02x)", e.Kind, uint8(e.Kind)))
	}
}

func (c *cli) EmitPlanLifecycle(e event.PlanEvent) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	switch e.Kind {
	case event.PlanStarted:
		c.commitRenderEvents(c.planRenderer.renderPlanStarted(e))
	case event.PlanFinished:
		c.commitRenderEvents(c.planRenderer.renderPlanFinished(e))
	case event.StepPlanned:
		c.commitRenderEvents(c.planRenderer.renderStepPlanned(e))
	case event.PlanProduced:
		c.commitRenderEvents(c.planRenderer.renderPlan(e))
	default:
		panic(errs.BUG("unknown plan event kind %q (0x%02x)", e.Kind, uint8(e.Kind)))
	}
}

func (c *cli) EmitActionLifecycle(e event.ActionEvent) {
	switch e.Kind {
	case event.ActionStarted:
		st := c.ensureActionFromStep(e.Step)
		if c.isTTY {
			c.render.sendLive(liveUpdate{
				kind: liveActionStarted,
				id:   st.id,
				desc: e.Step.StepDesc,
			})
		} else {
			c.commitRenderEvents([]renderEvent{{
				stream: streamOut,
				line:   c.formatter.fmtfMsg(colSpinner, "[%s] started...", st.id),
			}})
		}
	case event.ActionFinished:
		st := c.ensureActionFromStep(e.Step)
		c.render.sendLive(liveUpdate{
			kind: liveActionFinished,
			id:   st.id,
		})
		if !c.shouldRender(e.Chattiness) {
			return
		}
		c.commitRenderEvents(c.renderActionFinished(e))
	case event.HookTriggered:
		if !c.shouldRender(e.Chattiness) {
			return
		}
		c.commitRenderEvents(c.renderHookTriggered(e))
	case event.HookSkipped:
		if !c.shouldRender(e.Chattiness) {
			return
		}
	default:
		panic(errs.BUG("unknown action event kind %q (0x%02x)", e.Kind, uint8(e.Kind)))
	}
}

func (c *cli) EmitOpLifecycle(e event.OpEvent) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	switch e.Kind {
	case event.OpCheckStarted:
		// intentionally ignored
	case event.OpChecked:
		c.commitRenderEvents(c.renderOpChecked(e))
	case event.OpExecuteStarted:
		// intentionally ignored
	case event.OpExecuted:
		c.commitRenderEvents(c.renderOpExecuted(e))
	default:
		panic(errs.BUG("unknown op event kind %q (0x%02x)", e.Kind, uint8(e.Kind)))
	}
}

func (c *cli) EmitIndexAll(e event.IndexAllEvent) {
	if !c.shouldRender(e.Chattiness) {
		return
	}

	kindWidth := 0
	for _, step := range e.Steps {
		if w := layout.VisibleLen(step.Kind); w > kindWidth {
			kindWidth = w
		}
	}

	var events []renderEvent

	events = append(events, renderEvent{
		line:   c.formatter.fmtMsg(ansi.BrightBlack(), "AVAILABLE STEPS"),
		stream: streamOut,
	})
	events = append(events, renderEvent{line: "", stream: streamOut})

	for _, step := range e.Steps {
		kind := c.formatter.fmtMsg(ansi.BrightCyan().Bold(), step.Kind)
		pad := kindWidth - layout.VisibleLen(kind)
		if pad < 0 {
			pad = 0
		}
		prefix := "  " + kind + strings.Repeat(" ", pad) + "  "
		indent := 2 + kindWidth + 2

		descWidth := c.width - indent
		if descWidth <= 10 || c.width <= 0 {
			line := prefix + c.formatter.fmtMsg(ansi.White(), step.Desc)
			events = append(events, renderEvent{line: line, stream: streamOut, wrap: true})
		} else {
			for i, dl := range layout.WrapText(step.Desc, descWidth) {
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

func (c *cli) EmitInspect(e event.InspectEvent) {
	f := c.formatter
	var out []renderEvent

	header := f.fmtfMsg(
		ansi.Blue().Bold(),
		"deploy %q "+f.glyphs.arrow+" %s",
		e.Detail.DeployName,
		e.Detail.TargetName,
	)
	out = append(out,
		renderEvent{stream: streamOut, line: header},
		renderEvent{stream: streamOut, line: ""},
	)

	for _, entry := range e.Detail.Entries {
		line := "  " + f.fmtMsg(ansi.Cyan().Bold(), entry.Kind)
		if entry.Desc != "" {
			line += " " + f.fmtMsg(ansi.Cyan(), entry.Desc)
		}
		out = append(out, renderEvent{stream: streamOut, line: line})

		for _, field := range entry.Fields {
			label := f.fmtfMsg(ansi.BrightBlack(), "%-12s", field.Label)
			out = append(out, renderEvent{
				stream: streamOut,
				line:   "      " + label + " " + field.Value,
			})
		}
		out = append(out, renderEvent{stream: streamOut, line: ""})
	}

	c.commitRenderEvents(out)
}

func (c *cli) EmitIndexStep(e event.IndexStepEvent) {
	if !c.shouldRender(e.Chattiness) {
		return
	}

	doc := e.Doc
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

func (c *cli) renderFieldRows(fields []spec.FieldDoc) []renderEvent {
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

func (c *cli) renderFieldExamples(fields []spec.FieldDoc) []renderEvent {
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

func (c *cli) renderSnippets(snippets []string) []renderEvent {
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

func (c *cli) legendSection(header string, entries []legendEntry) []renderEvent {
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

func (c *cli) EmitLegend() {
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

func (c *cli) Interrupt() {
	c.render.interrupted.Store(true)
}

func (c *cli) Close() {
	c.render.close()
}

func (c *cli) renderEngineStarted(_ event.EngineEvent) []renderEvent {
	return []renderEvent{{
		stream: streamOut,
		line:   c.formatter.fmtfMsg(colEngineStarted, "[engine] started"),
	}}
}

func (c *cli) renderEngineConnecting(e event.EngineEvent) []renderEvent {
	d := e.ConnectingDetail
	return []renderEvent{{
		stream: streamOut,
		line:   c.formatter.fmtfMsg(colEngineStarted, "[engine] connecting to %s (%s)", d.TargetName, d.TargetKind),
	}}
}

func (c *cli) renderEngineFinished(e event.EngineEvent) []renderEvent {
	d := *e.Detail
	if d.Err != nil {
		return []renderEvent{{
			stream: streamErr,
			line: c.formatter.fmtfMsg(colEngineFinishedFatal,
				"[engine]%s failed: %v", glyphR(c.glyphs.fatal), d.Err),
		}}
	}

	if d.Cancelled {
		var parts []string
		parts = append(parts, fmt.Sprintf("%d changed", d.ChangedCount))
		parts = append(parts, fmt.Sprintf("%d step%s completed", d.TotalCount, layout.Plural(d.TotalCount)))
		parts = append(parts, d.Duration.String())
		summary := strings.Join(parts, ", ")
		return []renderEvent{{
			stream: streamOut,
			line:   c.formatter.fmtfMsg(colEngineFinishedChanged, "[engine] interrupted (%s)", summary),
		}}
	}

	color := colEngineFinishedUnchanged
	if d.FailedCount > 0 {
		color = colEngineFinishedFailed
	} else if d.ChangedCount > 0 || d.WouldChangeCount > 0 {
		color = colEngineFinishedChanged
	}

	var parts []string
	if d.CheckOnly {
		parts = append(parts, fmt.Sprintf("%d would change", d.WouldChangeCount))
	} else {
		parts = append(parts, fmt.Sprintf("%d changed", d.ChangedCount))
	}
	parts = append(parts, fmt.Sprintf("%d failed", d.FailedCount))
	parts = append(parts, fmt.Sprintf("%d hook%s fired", d.HooksFired, layout.Plural(d.HooksFired)))
	parts = append(parts, fmt.Sprintf("%d step%s", d.TotalCount, layout.Plural(d.TotalCount)))
	parts = append(parts, d.Duration.String())

	return []renderEvent{{
		stream: streamOut,
		line:   c.formatter.fmtfMsg(color, "[engine] finished (%s)", strings.Join(parts, ", ")),
	}}
}

func fmtSummary(s model.ActionSummary) string {
	switch {
	case s.Failed > 0 || s.Aborted > 0:
		return fmt.Sprintf("failed (%d failed, %d aborted)", s.Failed, s.Aborted)
	case s.Changed > 0:
		return fmt.Sprintf("%d/%d ops changed", s.Changed, s.Total)
	case s.WouldChange > 0:
		return fmt.Sprintf("%d/%d ops would change", s.WouldChange, s.Total)
	default:
		return "up-to-date"
	}
}

func (c *cli) summaryStyle(s model.ActionSummary) (ansi.ANSI, string) {
	switch {
	case s.Failed > 0 || s.Aborted > 0:
		return colActionFinishedFailed, c.glyphs.fatal
	case s.Changed > 0 || s.WouldChange > 0:
		return colActionFinishedChanged, c.glyphs.change
	default:
		return colActionFinishedUnchanged, c.glyphs.ok
	}
}

func (c *cli) renderActionFinished(e event.ActionEvent) []renderEvent {
	d := *e.Detail
	st := c.ensureActionFromStep(e.Step)
	color, glyph := c.summaryStyle(d.Summary)

	line := c.formatter.fmtfMsg(color, "[%s]%s", st.id, glyphR(glyph))
	if e.Step.StepDesc != "" {
		line += c.formatter.fmtfMsg(color, " %s %s", e.Step.StepDesc, c.glyphs.emDash)
	}
	line += c.formatter.fmtfMsg(color, " %s (%s)", fmtSummary(d.Summary), d.Duration)

	return []renderEvent{{stream: streamOut, line: line}}
}

func (c *cli) renderHookTriggered(e event.ActionEvent) []renderEvent {
	h := e.HookDetail
	color, glyph := c.summaryStyle(h.Summary)

	line := c.formatter.fmtfMsg(color, "[hook:%s]%s %s %s %s (%s)",
		h.HookID, glyphR(glyph), h.TriggerBy, c.glyphs.emDash, fmtSummary(h.Summary), h.Duration)

	return []renderEvent{{stream: streamOut, line: line}}
}

func (c *cli) renderOpChecked(e event.OpEvent) []renderEvent {
	d := *e.CheckDetail
	st := c.ensureActionFromStep(e.Step)

	switch d.Result {
	case spec.CheckSatisfied:
		events := []renderEvent{{
			stream: streamOut,
			line: c.formatter.fmtfMsg(colOpCheckSatisfied,
				"[%s]%s %s - up-to-date", st.id, glyphR(c.glyphs.ok), e.DisplayID),
		}}
		for _, dd := range d.Drift {
			if c.opts.Verbosity < dd.Verbosity {
				continue
			}
			events = append(events, renderEvent{
				stream: streamOut,
				line:   c.formatter.fmtfMsg(colOpDrift, "         %s: %s", dd.Field, dd.Current),
			})
		}
		return events
	case spec.CheckUnsatisfied:
		events := []renderEvent{{
			stream: streamOut,
			line: c.formatter.fmtfMsg(colOpCheckUnsatisfied,
				"[%s]%s %s - needs change", st.id, glyphR(c.glyphs.change), e.DisplayID),
		}}
		if c.opts.Verbosity >= signal.V && len(d.Drift) > 0 {
			for _, dd := range d.Drift {
				if c.opts.Verbosity < dd.Verbosity {
					continue
				}
				current := dd.Current
				if current == "" {
					current = "(missing)"
				}
				if dd.Desired == "" {
					events = append(events, renderEvent{
						stream: streamOut,
						line: c.formatter.fmtfMsg(colOpDrift,
							"         %s: %s", dd.Field, current),
					})
				} else {
					events = append(events, renderEvent{
						stream: streamOut,
						line: c.formatter.fmtfMsg(colOpDrift,
							"         %s: %s %s %s", dd.Field, current, c.glyphs.arrow, dd.Desired),
					})
				}
			}
		}
		return events
	case spec.CheckUnknown:
		return []renderEvent{{
			stream: streamErr,
			line: c.formatter.fmtfMsg(colOpCheckUnknown,
				"[%s]%s %s - unknown: %v", st.id, glyphR(c.glyphs.warn), e.DisplayID, d.Err),
		}}
	}
	return nil
}

func (c *cli) renderOpExecuted(e event.OpEvent) []renderEvent {
	d := *e.ExecuteDetail
	st := c.ensureActionFromStep(e.Step)

	switch {
	case d.Err != nil:
		return []renderEvent{{
			stream: streamErr,
			line: c.formatter.fmtfMsg(colOpExecFailed,
				"[%s]%s '%s' failed: %v", st.id, glyphR(c.glyphs.fatal), e.DisplayID, d.Err),
		}}
	case d.Changed:
		return []renderEvent{{
			stream: streamOut,
			line: c.formatter.fmtfMsg(colOpExecChanged,
				"[%s]%s '%s' changed (%s)", st.id, glyphR(c.glyphs.exec), e.DisplayID, d.Duration),
		}}
	default:
		return nil
	}
}

func (c *cli) EmitEngineDiagnostic(e event.EngineDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	context := ""
	if e.CfgPath != "" {
		context = fmt.Sprintf(` in file %q`, e.CfgPath)
	}
	c.renderDiagnostic(e.Severity, "engine", context, e.Detail.Template)
}

func (c *cli) EmitPlanDiagnostic(e event.PlanDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	c.renderDiagnostic(e.Severity, "plan", stepScope(e.Step), e.Detail.Template)
}

func (c *cli) EmitActionDiagnostic(e event.ActionDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	c.renderDiagnostic(e.Severity, "action", stepScope(e.Step), e.Detail.Template)
}

func (c *cli) EmitOpDiagnostic(e event.OpDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	c.renderDiagnostic(
		e.Severity,
		"op",
		fmt.Sprintf(` in op '%s' of%s`, e.DisplayID, stepScope(e.Step)),
		e.Detail.Template,
	)
}

func stepScope(s event.StepDetail) string {
	if s.StepIndex < 0 {
		if s.StepDesc != "" {
			return fmt.Sprintf(` in %s '%s'`, s.StepKind, s.StepDesc)
		}
		return ""
	}
	tag := fmt.Sprintf("%d", s.StepIndex)
	if s.StepKind != "" {
		tag += "|" + s.StepKind
	}
	if s.StepDesc != "" {
		return fmt.Sprintf(` in step [%s] '%s'`, tag, s.StepDesc)
	}
	return fmt.Sprintf(` in step [%s]`, tag)
}

func (c *cli) renderDiagnostic(sev signal.Severity, scope, msg string, tmpl event.Template) {
	var glyph, suffix string
	var col ansi.ANSI

	switch sev {
	case signal.Debug:
		glyph = c.glyphs.bullet
		col = colDiagDebug
		suffix = "debug"
	case signal.Info:
		glyph = c.glyphs.hint
		col = colDiagInfo
		suffix = "info"
	case signal.Notice:
		glyph = c.glyphs.ok
		col = colDiagNotice
		suffix = "notice"
	case signal.Warning:
		glyph = c.glyphs.warn
		col = colDiagWarning
		suffix = "warning"
	case signal.Error:
		glyph = c.glyphs.err
		col = colDiagError
		suffix = "error"
	case signal.Fatal:
		glyph = c.glyphs.fatal
		col = colDiagFatal
		suffix = "fatal"
	default:
		panic(errs.BUG("unhandled diagnostic severity %d", sev))
	}

	label := scope + "." + suffix

	var events []renderEvent
	for _, l := range c.formatter.fmtTemplate(tmpl, label, msg, glyph, col, colDiagHelp) {
		events = append(events, renderEvent{stream: streamErr, line: l, wrap: true})
	}
	c.commitRenderEvents(events)
}

func (c *cli) ensureActionFromStep(step event.StepDetail) *actionState {
	var key string
	if step.HookID != "" {
		key = fmt.Sprintf("hook:%s][%s", step.HookID, step.StepKind)
	} else {
		key = fmt.Sprintf("%s:%d", step.StepKind, step.StepIndex)
	}
	st, _ := c.actions.LoadOrStore(key, &actionState{id: key})
	return st.(*actionState)
}

func (c *cli) shouldUseColor() bool {
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
