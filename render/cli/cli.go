package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/term"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/model"
	"godoit.dev/doit/render"
	"godoit.dev/doit/render/ansi"
	"godoit.dev/doit/render/layout"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
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
	store   *spec.SourceStore
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
func New(opts Options, store *spec.SourceStore) render.Displayer {
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
		render:       newRenderer(os.Stdout, os.Stderr),
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
		events[i].line = layout.FitLine(events[i].line, c.width)
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
	if !c.shouldRender(e.Chattiness) {
		return
	}
	switch e.Kind {
	case event.ActionStarted:
		// intentionally ignored
	case event.ActionFinished:
		c.commitRenderEvents(c.renderActionFinished(e))
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
		desc := c.formatter.fmtMsg(ansi.White(), step.Desc)
		pad := kindWidth - layout.VisibleLen(kind)
		if pad < 0 {
			pad = 0
		}
		line := "  " + kind + strings.Repeat(" ", pad) + "  " + desc
		events = append(events, renderEvent{line: line, stream: streamOut})
	}

	events = append(events, renderEvent{line: "", stream: streamOut})
	events = append(events, renderEvent{
		line:   c.formatter.fmtMsg(ansi.BrightBlack(), "Use 'doit index <step>' for details."),
		stream: streamOut,
	})

	c.commitRenderEvents(events)
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
		events = append(events, renderEvent{
			line:   c.formatter.fmtMsg(ansi.White(), "  "+doc.Summary),
			stream: streamOut,
		})
	}

	if len(doc.Fields) > 0 {
		events = append(events, renderEvent{line: "", stream: streamOut})
		events = append(events, renderEvent{
			line:   c.formatter.fmtMsg(ansi.BrightBlack(), "FIELDS"),
			stream: streamOut,
		})
		events = append(events, renderEvent{line: "", stream: streamOut})

		nameW, typeW, reqW := 0, 0, 8
		for _, f := range doc.Fields {
			if len(f.Name) > nameW {
				nameW = len(f.Name)
			}
			if len(f.Type) > typeW {
				typeW = len(f.Type)
			}
		}

		for _, f := range doc.Fields {
			reqStr := "optional"
			if f.Required {
				reqStr = "required"
			}
			line := fmt.Sprintf("  %-*s   %-*s   %-*s   %s",
				nameW, f.Name, typeW, f.Type, reqW, reqStr, f.Desc)
			events = append(events, renderEvent{
				line:   c.formatter.fmtMsg(ansi.White(), line),
				stream: streamOut,
			})
		}
	}

	if len(doc.Examples) > 0 && c.opts.Verbosity >= signal.V {
		showAll := len(doc.Examples) == 1 || c.opts.Verbosity >= signal.VV
		visible := doc.Examples
		if !showAll {
			visible = doc.Examples[:1]
		}

		header := "EXAMPLE" + strings.ToUpper(layout.Plural(len(visible)))
		events = append(events, renderEvent{line: "", stream: streamOut})
		events = append(events, renderEvent{
			line:   c.formatter.fmtMsg(ansi.BrightBlack(), header),
			stream: streamOut,
		})
		events = append(events, renderEvent{line: "", stream: streamOut})

		for _, example := range visible {
			for l := range strings.SplitSeq(example, "\n") {
				events = append(events, renderEvent{
					line:   c.formatter.fmtMsg(ansi.BrightBlue(), "  "+l),
					stream: streamOut,
				})
			}
		}

		if !showAll {
			events = append(events, renderEvent{line: "", stream: streamOut})
			events = append(events, renderEvent{
				line: c.formatter.fmtMsg(ansi.BrightBlack(),
					fmt.Sprintf("Use -vv to see all %d examples.", len(doc.Examples))),
				stream: streamOut,
			})
		}
	} else if len(doc.Examples) > 0 {
		events = append(events, renderEvent{line: "", stream: streamOut})
		events = append(events, renderEvent{
			line:   c.formatter.fmtMsg(ansi.BrightBlack(), "Use -v to see examples."),
			stream: streamOut,
		})
	}

	c.commitRenderEvents(events)
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

	planBoundary := fmt.Sprintf("%s ··· %s", c.glyphs.planStart, c.glyphs.planEnd)
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
		{"← [N, ...]", colPlanDeps, "", "depends on action N (must complete first)"},
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

func (c *cli) Close() {
	c.render.close()
}

func (c *cli) renderEngineStarted(_ event.EngineEvent) []renderEvent {
	return []renderEvent{{
		stream: streamOut,
		line:   c.formatter.fmtfMsg(colEngineStarted, "[engine] started"),
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
		parts = append(parts, fmt.Sprintf("%d change%s", d.ChangedCount, layout.Plural(d.ChangedCount)))
	}
	parts = append(parts, fmt.Sprintf("%d failure%s", d.FailedCount, layout.Plural(d.FailedCount)))
	parts = append(parts, fmt.Sprintf("%d step%s", d.TotalCount, layout.Plural(d.TotalCount)))
	parts = append(parts, d.Duration.String())

	return []renderEvent{{
		stream: streamOut,
		line:   c.formatter.fmtfMsg(color, "[engine] finished (%s)", strings.Join(parts, ", ")),
	}}
}

func (c *cli) renderActionFinished(e event.ActionEvent) []renderEvent {
	fmtActionSummary := func(s model.ActionSummary) string {
		switch {
		case s.Failed > 0 || s.Aborted > 0:
			return fmt.Sprintf("failed after %d/%d ops (%d failed, %d aborted)",
				s.Succeeded+s.Failed, s.Total, s.Failed, s.Aborted)
		case s.Changed > 0:
			return fmt.Sprintf("%d/%d ops changed", s.Changed, s.Total)
		case s.WouldChange > 0:
			return fmt.Sprintf("%d/%d ops would change", s.WouldChange, s.Total)
		default:
			return "up-to-date"
		}
	}

	d := *e.Detail
	smry := d.Summary
	st := c.ensureActionFromStep(e.Step)

	var color ansi.ANSI
	var glyph string

	switch {
	case smry.Failed > 0 || smry.Aborted > 0:
		color = colActionFinishedFailed
		glyph = c.glyphs.fatal
	case smry.Changed > 0 || smry.WouldChange > 0:
		color = colActionFinishedChanged
		glyph = c.glyphs.change
	default:
		color = colActionFinishedUnchanged
		glyph = c.glyphs.ok
	}

	line := c.formatter.fmtfMsg(color, "[%s]%s", st.id, glyphR(glyph))
	if e.Step.StepDesc != "" {
		line += c.formatter.fmtfMsg(color, " %s —", e.Step.StepDesc)
	}
	line += c.formatter.fmtfMsg(color, " %s (%s)", fmtActionSummary(smry), d.Duration)

	return []renderEvent{{stream: streamOut, line: line}}
}

func (c *cli) renderOpChecked(e event.OpEvent) []renderEvent {
	d := *e.CheckDetail
	st := c.ensureActionFromStep(e.Step)

	switch d.Result {
	case spec.CheckSatisfied:
		return []renderEvent{{
			stream: streamOut,
			line: c.formatter.fmtfMsg(colOpCheckSatisfied,
				"[%s]%s %s - up-to-date", st.id, glyphR(c.glyphs.ok), e.DisplayID),
		}}
	case spec.CheckUnsatisfied:
		events := []renderEvent{{
			stream: streamOut,
			line: c.formatter.fmtfMsg(colOpCheckUnsatisfied,
				"[%s]%s %s - needs change", st.id, glyphR(c.glyphs.change), e.DisplayID),
		}}
		if c.opts.Verbosity >= signal.V && len(d.Drift) > 0 {
			for _, dd := range d.Drift {
				current := dd.Current
				if current == "" {
					current = "(missing)"
				}
				events = append(events, renderEvent{
					stream: streamOut,
					line: c.formatter.fmtfMsg(colOpDrift,
						"         %s: %s %s %s", dd.Field, current, c.glyphs.arrow, dd.Desired),
				})
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
	c.renderDiagnostic("engine.error", context, e.Detail.Template)
}

func (c *cli) EmitPlanDiagnostic(e event.PlanDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	c.renderDiagnostic("plan.error",
		fmt.Sprintf(` in step [%d|%s] '%s'`, e.Step.StepIndex, e.Step.StepKind, e.Step.StepDesc),
		e.Detail.Template)
}

func (c *cli) EmitActionDiagnostic(e event.ActionDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	c.renderDiagnostic("action.error",
		fmt.Sprintf(` in step [%d|%s] '%s'`, e.Step.StepIndex, e.Step.StepKind, e.Step.StepDesc),
		e.Detail.Template)
}

func (c *cli) EmitOpDiagnostic(e event.OpDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}
	c.renderDiagnostic("op.error",
		fmt.Sprintf(` in op '%s' of step [%d|%s] '%s'`,
			e.DisplayID, e.Step.StepIndex, e.Step.StepKind, e.Step.StepDesc),
		e.Detail.Template)
}

func (c *cli) renderDiagnostic(prefix, msg string, tmpl event.Template) {
	var events []renderEvent
	for _, l := range c.formatter.fmtTemplate(tmpl, prefix, msg, c.glyphs.err, colDiagMsg, colDiagHelp) {
		events = append(events, renderEvent{stream: streamErr, line: l})
	}
	c.commitRenderEvents(events)
}

func (c *cli) ensureActionFromStep(step event.StepDetail) *actionState {
	key := fmt.Sprintf("%s:%d", step.StepKind, step.StepIndex)
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
