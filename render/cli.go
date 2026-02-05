//go:generate stringer -type=stream
package render

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/model"
	"godoit.dev/doit/render/ansi"
	"godoit.dev/doit/render/template"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type (
	CLIOptions struct {
		ColorMode  signal.ColorMode
		Verbosity  signal.Verbosity
		ForceASCII bool
	}

	cli struct {
		opts    CLIOptions
		render  *renderer
		glyphs  glyphSet
		store   *spec.SourceStore
		actions sync.Map // map[string]*actionState

		// layout
		isTTY bool
		width int
	}
	actionState struct {
		id string
	}
	sourceLine struct {
		filename string
		line     int
		startCol int
		endCol   int
		text     string
		ok       bool
	}
	glyphSet struct {
		change string
		ok     string
		exec   string
		warn   string
		error  string
		fatal  string
		hint   string
		help   string
		bullet string

		// plan rails
		planStart            string
		planRail             string
		planEnd              string
		actionStart          string
		actionStartNoOp      string
		actionStartCollapsed string
		actionRail           string
		actionIndent         string
		actionKindSep        string
		actionEnd            string
		opBranch             string
		opLast               string

		// parallel layer brackets (right-side)
		parallelTop   string
		parallelMid   string
		parallelBot   string
		parallelLabel string
	}
)

const (
	minWidePlanCols = 70 // below this, fancy, wide plan rendering adds more noise than clarity
)

var (
	colEngineStarted           = ansi.Green().Dim()
	colEngineFinishedUnchanged = ansi.Green()
	colEngineFinishedChanged   = ansi.Yellow()
	colEngineFinishedFailed    = ansi.Red()
	colEngineFinishedFatal     = ansi.BrightRed().Bold()

	colPlanHeader             = ansi.Magenta().Bold()
	colPlanRail               = ansi.Magenta().Dim()
	colPlanStarted            = ansi.Blue()
	colPlanStartedUnit        = ansi.Blue().Bold()
	colPlanFinishedOK         = ansi.Blue().Dim()
	colPlanFinishedOKUnit     = ansi.Blue().Dim().Bold()
	colPlanFinishedFailed     = ansi.Red()
	colPlanFinishedFailedUnit = ansi.Red().Bold()
	colPlanStepPlanned        = ansi.BrightBlack().Dim()

	colActionKind              = ansi.Cyan().Bold()
	colActionDesc              = ansi.Cyan()
	colActionRail              = ansi.Cyan()
	colActionOps               = ansi.Cyan().Dim()
	colActionFinishedUnchanged = ansi.Green().Dim()
	colActionFinishedChanged   = ansi.Yellow()
	colActionFinishedFailed    = ansi.Red()

	colOpHeader           = ansi.BrightBlack()
	colOpRail             = ansi.BrightBlack().Dim()
	colOpDesc             = ansi.BrightBlack().Dim()
	colOpCheckSatisfied   = ansi.BrightBlack().Dim()
	colOpCheckUnsatisfied = ansi.BrightBlack().Dim()
	colOpCheckUnknown     = ansi.Yellow()
	colOpExecChanged      = ansi.BrightBlack()
	colOpExecFailed       = ansi.Red()

	colPlanDeps    = ansi.BrightBlack().Dim()
	colPlanBracket = ansi.BrightBlack().Dim()

	colDiagMsg      = ansi.Red()
	colDiagHelp     = ansi.Cyan()
	colSourceGutter = ansi.BrightBlack()
	colSourceCaret  = ansi.Red()
)

var (
	fancyGlyphs = glyphSet{
		// Nerdfont state glyphs
		// ===============================================
		// '󰏫' nf-md-pencil
		// '󰄬' nf-md-check
		// '󰄭' nf-md-check_all
		// '󰗠' nf-md-check_circle
		// '󰒓' nf-md-cog
		// '󰼛' nf-md-play_outline
		// '󰐊' nf-md-play
		// '󰀦' nf-md-alert
		// '󰀪' nf-md-alert_outline
		// '󰈅' nf-md-exclamation
		// '󰚌' nf-md-skull
		// '󰯈' nf-md-skull_outline
		// '󰅖' nf-md-close
		// '󰌵' nf-md-lightbulb
		// '󰌶' nf-md-lightbulb_outline
		// '󰋖' nf-md-help
		// '󰋗' nf-md-help_circle
		// '󰘥' nf-md-help_circle_outline
		// '󰡾' nf-md-lifebuoy
		// '󰂭' nf-md-block_helper
		change: "󰏫",
		ok:     "󰄬",
		exec:   "󰐊",
		warn:   "󰀦",
		error:  "󰅖",
		fatal:  "󰚌",
		hint:   "󰌵",
		help:   "󰋖",
		bullet: "•",

		// plan rails
		planStart:            "┌─┬",
		planRail:             "│",
		planEnd:              "└─■",
		actionStart:          "┏━┯",
		actionStartNoOp:      "┏━━",
		actionStartCollapsed: "•",
		actionRail:           "┇",
		actionIndent:         "  ",
		actionKindSep:        "›",
		actionEnd:            "■",
		opBranch:             "├─",
		opLast:               "└─",

		parallelTop:   "╮",
		parallelMid:   "│",
		parallelBot:   "╯",
		parallelLabel: "⏸",
	}

	asciiGlyphs = glyphSet{
		// State glyphs (ASCII-safe, semantic)
		change: "~",
		ok:     "+",
		exec:   ">",
		warn:   "!",
		error:  "x",
		fatal:  "X",
		hint:   "?",
		help:   "i",
		bullet: "*",

		// Plan rails
		planStart:            "+--",
		planRail:             "|",
		planEnd:              "+-#",
		actionStart:          "+--",
		actionStartNoOp:      "+--",
		actionStartCollapsed: "*",
		actionRail:           "|",
		actionIndent:         "  ",
		actionKindSep:        ">",
		actionEnd:            "#",
		opBranch:             "|-",
		opLast:               "`-",

		parallelTop:   ")",
		parallelMid:   ")",
		parallelBot:   ")",
		parallelLabel: "\"",
	}
)

func NewCLI(opts CLIOptions, store *spec.SourceStore) Displayer {
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

	return &cli{
		opts:  opts,
		store: store,
		render: newRenderer(
			os.Stdout,
			os.Stderr,
		),
		glyphs: glyphs,
		isTTY:  term.IsTerminal(os.Stdout.Fd()),
		width:  width,
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
	// fit lines before committing to draw loop
	for i := range events {
		if strings.ContainsAny(events[i].line, "\n\r") {
			panic(errs.BUG("renderEvent.line must neither contain '\\n' nor '\\r'"))
		}
		events[i].line = fitLine(events[i].line, c.width)
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
		c.commitRenderEvents(c.renderPlanStarted(e))
	case event.PlanFinished:
		c.commitRenderEvents(c.renderPlanFinished(e))
	case event.StepPlanned:
		c.commitRenderEvents(c.renderStepPlanned(e))
	case event.PlanProduced:
		c.commitRenderEvents(c.renderPlan(e))
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
		panic(errs.BUG("OP LC %q", e.Kind))

	}
}

func (c *cli) EmitIndexAll(e event.IndexAllEvent) {
	if !c.shouldRender(e.Chattiness) {
		return
	}

	maxKindWidth := func(steps []event.StepIndexDetail) int {
		maxLen := 0
		for _, s := range steps {
			if w := utf8.RuneCountInString(s.Kind); w > maxLen {
				maxLen = w
			}
		}
		return maxLen
	}
	ansiRE := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	visibleLen := func(s string) int {
		return utf8.RuneCountInString(ansiRE.ReplaceAllString(s, ""))
	}

	padRight := func(s string, width int) string {
		n := width - visibleLen(s)
		if n <= 0 {
			return s
		}
		return s + strings.Repeat(" ", n)
	}

	kindWidth := maxKindWidth(e.Steps)

	var events []renderEvent

	// Header
	events = append(events, renderEvent{
		line:   c.fmtMsg(ansi.BrightBlack(), "AVAILABLE STEPS"),
		stream: streamOut,
	})
	events = append(events, renderEvent{
		line:   "",
		stream: streamOut,
	})

	// Step list with indentation
	for _, step := range e.Steps {
		kind := c.fmtMsg(ansi.BrightCyan().Bold(), step.Kind)
		desc := c.fmtMsg(ansi.White(), step.Desc)

		line := fmt.Sprintf("  %s  %s", padRight(kind, kindWidth), desc)

		events = append(events, renderEvent{
			line:   line,
			stream: streamOut,
		})
	}

	// Footer hint
	events = append(events, renderEvent{
		line:   "",
		stream: streamOut,
	})
	events = append(events, renderEvent{
		line:   c.fmtMsg(ansi.BrightBlack(), "Use 'doit index <step>' for details."),
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

	// Header: step name
	events = append(events, renderEvent{
		line:   c.fmtMsg(ansi.BrightCyan().Bold(), strings.ToUpper(doc.Kind)),
		stream: streamOut,
	})

	// Summary
	if doc.Summary != "" {
		events = append(events, renderEvent{
			line:   "",
			stream: streamOut,
		})
		events = append(events, renderEvent{
			line:   c.fmtMsg(ansi.White(), "  "+doc.Summary),
			stream: streamOut,
		})
	}

	// Fields section
	if len(doc.Fields) > 0 {
		events = append(events, renderEvent{
			line:   "",
			stream: streamOut,
		})
		events = append(events, renderEvent{
			line:   c.fmtMsg(ansi.BrightBlack(), "FIELDS"),
			stream: streamOut,
		})
		events = append(events, renderEvent{
			line:   "",
			stream: streamOut,
		})

		// Calculate column widths for alignment
		nameW, typeW, reqW := 0, 0, 8 // "required" or "optional"
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
				nameW, f.Name,
				typeW, f.Type,
				reqW, reqStr,
				f.Desc,
			)
			events = append(events, renderEvent{
				line:   c.fmtMsg(ansi.White(), line),
				stream: streamOut,
			})
		}
	}

	// Examples (only with -v, for now we always show if present)
	if len(doc.Examples) > 0 && c.opts.Verbosity >= signal.V {
		events = append(events, renderEvent{
			line:   "",
			stream: streamOut,
		})
		events = append(events, renderEvent{
			line:   c.fmtMsg(ansi.BrightBlack(), "EXAMPLE"),
			stream: streamOut,
		})
		events = append(events, renderEvent{
			line:   "",
			stream: streamOut,
		})

		for _, example := range doc.Examples {
			for l := range strings.SplitSeq(example, "\n") {
				events = append(events, renderEvent{
					line:   c.fmtMsg(ansi.BrightBlue(), "  "+l),
					stream: streamOut,
				})
			}
		}
	} else if len(doc.Examples) > 0 {
		events = append(events, renderEvent{
			line:   "",
			stream: streamOut,
		})
		events = append(events, renderEvent{
			line:   c.fmtMsg(ansi.BrightBlack(), "Use -v to see examples."),
			stream: streamOut,
		})
	}

	c.commitRenderEvents(events)
}

type legendEntry struct {
	glyph string
	color ansi.ANSI
	desc  string
}

func (c *cli) legendSection(header string, entries []legendEntry) []renderEvent {
	var events []renderEvent

	events = append(events, renderEvent{
		stream: streamOut,
		line:   c.fmtMsg(ansi.BrightBlack(), header),
	})
	events = append(events, renderEvent{
		stream: streamOut,
		line:   "",
	})

	maxGlyphWidth := 0
	for _, e := range entries {
		if w := visibleLen(c.fmtMsg(e.color, e.glyph)); w > maxGlyphWidth {
			maxGlyphWidth = w
		}
	}

	for _, e := range entries {
		if e.glyph == "" && e.desc == "" {
			events = append(events, renderEvent{
				stream: streamOut,
				line:   "",
			})
			continue
		}

		styled := c.fmtMsg(e.color, e.glyph)
		pad := maxGlyphWidth - visibleLen(styled)
		if pad < 0 {
			pad = 0
		}
		line := "  " + styled + strings.Repeat(" ", pad)
		if e.desc != "" {
			line += "  " + c.fmtMsg(ansi.White(), e.desc)
		}
		events = append(events, renderEvent{
			stream: streamOut,
			line:   line,
		})
	}

	return events
}

func (c *cli) EmitLegend() {
	var events []renderEvent

	// ─── STATE ───
	events = append(events, c.legendSection("STATE", []legendEntry{
		{c.glyphs.change, colActionFinishedChanged, "change    system state was modified"},
		{c.glyphs.ok, colActionFinishedUnchanged, "ok        already correct, no change needed"},
		{c.glyphs.exec, colOpExecChanged, "exec      operation executed"},
		{c.glyphs.warn, colOpCheckUnknown, "warn      non-fatal issue"},
		{c.glyphs.error, colOpExecFailed, "error     operation failed"},
		{c.glyphs.fatal, colEngineFinishedFatal, "fatal     unrecoverable failure"},
	})...)
	events = append(events, renderEvent{stream: streamOut, line: ""})

	// ─── PLAN ───
	planBoundary := fmt.Sprintf(
		"%s ··· %s",
		c.glyphs.planStart,
		c.glyphs.planEnd,
	)
	actionHeader := fmt.Sprintf(
		"%s [0] copy",
		c.glyphs.actionStart,
	)
	opBranch := fmt.Sprintf(
		"%s %s CopyCheck",
		c.glyphs.actionRail,
		c.glyphs.opBranch,
	)
	opLast := fmt.Sprintf(
		"%s %s CopyExec",
		c.glyphs.actionRail,
		c.glyphs.opLast,
	)
	collapsed := fmt.Sprintf(
		"%s  [2] symlink",
		c.glyphs.actionStartCollapsed,
	)

	events = append(events, c.legendSection("PLAN", []legendEntry{
		{planBoundary, colPlanRail, "plan boundary (wraps entire execution)"},
		{c.glyphs.planRail, colPlanRail, "plan rail (actions listed inside)"},
		{"", ansi.ANSI{}, ""},
		{actionHeader, colActionKind, "action start (step with ops)"},
		{opBranch, colOpHeader, "op branch"},
		{opLast, colOpHeader, "op branch (last)"},
		{c.glyphs.actionEnd, colActionRail, "action end"},
		{"", ansi.ANSI{}, ""},
		{collapsed, colActionKind, "collapsed action (default verbosity)"},
		{"", ansi.ANSI{}, ""},
		{"← [N, ...]", colPlanDeps, "depends on action N (must complete first)"},
		{c.glyphs.parallelTop, colPlanBracket, "parallel execution group"},
		{c.glyphs.parallelMid, colPlanBracket, ""},
		{c.glyphs.parallelBot, colPlanBracket, ""},
		{c.glyphs.parallelLabel, colPlanBracket, "group boundary (engine waits for all)"},
	})...)
	events = append(events, renderEvent{stream: streamOut, line: ""})

	// ─── COLORS (only meaningful when color is active) ───
	if c.shouldUseColor() {
		events = append(events, c.legendSection("COLORS", []legendEntry{
			{"yellow", ansi.Yellow(), "mutation, system state changed"},
			{"green", ansi.Green(), "correct, no change needed"},
			{"red", ansi.Red(), "failure"},
			{"blue", ansi.Blue(), "engine and plan boundaries"},
			{"magenta", ansi.Magenta(), "plan structure"},
			{"cyan", ansi.Cyan(), "action context"},
			{"dim", ansi.BrightBlack().Dim(), "detail (higher verbosity)"},
		})...)
		events = append(events, renderEvent{stream: streamOut, line: ""})
	}

	c.commitRenderEvents(events)
}

func (c *cli) Close() {
	c.render.close()
}

// Engine lifecycle
// ===============================================

func (c *cli) renderEngineStarted(_ event.EngineEvent) []renderEvent {
	return []renderEvent{{
		stream: streamOut,
		line: c.fmtfMsg(
			colEngineStarted,
			"[engine] started",
		),
	}}
}

func (c *cli) renderEngineFinished(e event.EngineEvent) []renderEvent {
	d := *e.Detail
	if d.Err != nil {
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtfMsg(
				colEngineFinishedFatal,
				"[engine]%s failed: %v",
				glyphr(c.glyphs.fatal),
				d.Err,
			),
		}}
	}

	color := colEngineFinishedUnchanged
	if d.FailedCount > 0 {
		color = colEngineFinishedFailed
	} else if d.ChangedCount > 0 || d.WouldChangeCount > 0 {
		color = colEngineFinishedChanged
	}

	// Build summary parts
	var parts []string
	if d.CheckOnly {
		// Check mode: always report "would change" count
		parts = append(parts, fmt.Sprintf("%d would change", d.WouldChangeCount))
	} else {
		// Apply mode: report actual changes
		parts = append(parts, fmt.Sprintf("%d change%s", d.ChangedCount, s(d.ChangedCount)))
	}
	parts = append(parts, fmt.Sprintf("%d failure%s", d.FailedCount, s(d.FailedCount)))
	parts = append(parts, fmt.Sprintf("%d step%s", d.TotalCount, s(d.TotalCount)))
	parts = append(parts, d.Duration.String())

	return []renderEvent{{
		stream: streamOut,
		line: c.fmtfMsg(
			color,
			"[engine] finished (%s)",
			strings.Join(parts, ", "),
		),
	}}
}

// Plan lifecycle
// ===============================================

func (c *cli) renderPlanStarted(e event.PlanEvent) []renderEvent {
	d := *e.StartedDetail
	line := c.fmtfMsg(
		colPlanStarted,
		"[plan] planning",
	)
	if d.UnitID != "" {
		line += c.fmtfMsg(
			colPlanStartedUnit,
			" %s",
			d.UnitID,
		)
	}
	line += c.fmtMsg(
		colPlanStarted,
		" started",
	)

	return []renderEvent{{
		stream: streamOut,
		line:   line,
	}}
}

func (c *cli) renderPlanFinished(e event.PlanEvent) []renderEvent {
	d := *e.FinishedDetail

	var events []renderEvent

	ttlSteps := d.SuccessfulSteps + d.FailedSteps
	if d.FailedSteps > 0 {
		line := c.fmtfMsg(
			colPlanFinishedFailed,
			"[plan]%s planning",
			glyphr(c.glyphs.fatal),
		)
		if d.UnitID != "" {
			line += c.fmtfMsg(
				colPlanFinishedFailedUnit,
				" %s",
				d.UnitID,
			)
		}
		line += c.fmtfMsg(
			colPlanFinishedFailed,
			" aborted: %d/%d step%s planned, %d/%d step%s failed (%s)",
			d.SuccessfulSteps,
			ttlSteps,
			s(d.SuccessfulSteps),
			d.FailedSteps,
			ttlSteps,
			s(d.FailedSteps),
			d.Duration,
		)

		events = append(events, renderEvent{
			stream: streamOut,
			line:   line,
		})
	} else {
		line := c.fmtMsg(
			colPlanFinishedOK,
			"[plan] planning",
		)

		if d.UnitID != "" {
			line += c.fmtfMsg(
				colPlanFinishedOKUnit,
				" %s",
				d.UnitID,
			)
		}
		line += c.fmtfMsg(
			colPlanFinishedOK,
			" finished: %d step%s planned (%s)",
			d.SuccessfulSteps,
			s(d.SuccessfulSteps),
			d.Duration,
		)

		events = append(events, renderEvent{
			stream: streamOut,
			line:   line,
		})
	}

	return events
}

func (c *cli) renderStepPlanned(e event.PlanEvent) []renderEvent {
	return []renderEvent{{
		stream: streamOut,
		line: c.fmtfMsg(
			colPlanStepPlanned,
			"[plan.step]%s #%d %s '%s'",
			glyphr(c.glyphs.ok),
			e.Step.StepIndex,
			e.Step.StepKind,
			e.Step.StepDesc,
		),
	}}
}

// renderPlan invariant:
// The plan is represented as a single continuous vertical rail.
// Action rails are nested inside the plan rail.
// Ops never touch the plan rail directly.
func (c *cli) renderPlan(e event.PlanEvent) []renderEvent {
	d := *e.Detail

	if c.width < minWidePlanCols {
		for i := range d.Actions {
			for j := range d.Actions[i].Ops {
				if tmpl := d.Actions[i].Ops[j].Template; tmpl != nil {
					tmpl.Text = ""
				}
			}
		}
	}

	var out []renderEvent
	v := c.opts.Verbosity

	out = append(out, renderEvent{
		stream: streamOut,
		line:   "",
	})

	hdr := c.fmtMsg(colPlanRail, c.glyphs.planStart)
	hdr += c.fmtMsg(
		colPlanHeader,
		" execution plan",
	)

	if d.UnitID != "" {
		hdr += c.fmtfMsg(
			colPlanHeader,
			": %s",
			d.UnitID,
		)
		if d.UnitDesc != "" {
			hdr += c.fmtfMsg(
				colPlanHeader,
				" %s %s",
				c.glyphs.actionKindSep, d.UnitDesc,
			)
		}
	}

	out = append(out, renderEvent{
		stream: streamOut,
		line:   hdr,
	})

	dag := buildPlanDAG(d)

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
	for _, layer := range dag.ActionLayers {
		for _, act := range layer {
			if act.Index > maxIndex {
				maxIndex = act.Index
			}
		}
	}

	indexWidth := digits10(maxIndex)

	// ─── Pass 1: build inner lines for each action ───

	type actionEntry struct {
		innerLines []string // lines without plan rail prefix
		headerIdx  int      // index of the header line
		layerSize  int      // number of actions in this layer
		posInLayer int      // 0-based position within layer
		deps       []int    // action dependencies
	}

	var actions []actionEntry

	for _, layer := range dag.ActionLayers {
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
				desc = fmt.Sprintf(" %s %s", c.glyphs.actionKindSep, act.Desc)
			}

			if v == signal.Quiet {
				nOps := 0
				for _, l := range act.Layers {
					nOps += len(l)
				}

				line := c.fmtMsg(colActionKind, " "+c.glyphs.actionStartCollapsed) +
					c.fmtfMsg(colActionKind, " [%*d]%s", indexWidth, act.Index, kind)
				if desc != "" {
					line += c.fmtMsg(colActionDesc, desc)
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
				line += c.fmtMsg(colActionOps, opLine)

				ae.innerLines = []string{line}
			} else {
				gutter := c.glyphs.actionStart
				if len(act.Layers) == 0 {
					gutter = c.glyphs.actionStartNoOp
				}

				headerLine := " " + c.fmtMsg(colActionRail, gutter) +
					c.fmtfMsg(colActionKind, " [%*d]%s", indexWidth, act.Index, kind)
				if desc != "" {
					headerLine += c.fmtMsg(colActionDesc, desc)
				}

				ae.innerLines = []string{headerLine}

				ops := flattenLayers(act.Layers)
				children := buildDepTree(ops)
				roots := findRoots(ops)

				for i, root := range roots {
					c.collectOpTreeLines(
						&ae.innerLines,
						root, children,
						[]bool{true},
						i == len(roots)-1,
						v,
					)
				}

				ae.innerLines = append(ae.innerLines,
					" "+c.fmtMsg(colActionRail, c.glyphs.actionEnd))
			}

			actions = append(actions, ae)
		}
	}

	// ─── Pass 2: compute alignment ───

	maxHeaderWidth := 0
	for _, ae := range actions {
		if w := visibleLen(ae.innerLines[ae.headerIdx]); w > maxHeaderWidth {
			maxHeaderWidth = w
		}
	}

	// Build deps strings (plain text, no ANSI)
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

	// Bracket column (relative to inner content start).
	// Must be past both (a) header + deps annotations and
	// (b) the widest line in any parallel-layer action (op-tree
	// lines in -vv mode include template descriptions).
	hasParallel := false
	maxParLineWidth := 0
	for _, ae := range actions {
		if ae.layerSize > 1 {
			hasParallel = true
			for _, line := range ae.innerLines {
				if w := visibleLen(line); w > maxParLineWidth {
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

	// ─── Pass 3: emit annotated lines ───

	rail := c.fmtMsg(colPlanRail, c.glyphs.planRail)

	for i, ae := range actions {
		lastLineIdx := len(ae.innerLines) - 1

		for lineIdx, innerLine := range ae.innerLines {
			isHeader := lineIdx == ae.headerIdx
			fullLine := rail + innerLine

			if isHeader {
				// Pad header to alignment column
				if pad := maxHeaderWidth - visibleLen(innerLine); pad > 0 {
					fullLine += strings.Repeat(" ", pad)
				}

				// Append deps annotation
				if depsStrs[i] != "" {
					fullLine += c.fmtMsg(colPlanDeps, "  "+depsStrs[i])
				}
			}

			// Bracket glyph for parallel layers — spans the full group
			// from first action header (╮ ⏸) to last action's final line (╯).
			if ae.layerSize > 1 {
				var currentWidth int
				if isHeader {
					currentWidth = maxHeaderWidth
					if depsStrs[i] != "" {
						currentWidth += 2 + len(depsStrs[i])
					}
				} else {
					currentWidth = visibleLen(innerLine)
				}

				pad := bracketCol - currentWidth
				if pad < 1 {
					pad = 1
				}
				fullLine += strings.Repeat(" ", pad)

				switch {
				case ae.posInLayer == 0 && isHeader:
					fullLine += c.fmtMsg(colPlanBracket,
						c.glyphs.parallelTop+" "+c.glyphs.parallelLabel)
				case ae.posInLayer == ae.layerSize-1 && lineIdx == lastLineIdx:
					fullLine += c.fmtMsg(colPlanBracket, c.glyphs.parallelBot)
				default:
					fullLine += c.fmtMsg(colPlanBracket, c.glyphs.parallelMid)
				}
			}

			out = append(out, renderEvent{
				stream: streamOut,
				line:   fullLine,
			})
		}
	}

	out = append(out, renderEvent{
		stream: streamOut,
		line:   c.fmtMsg(colPlanRail, c.glyphs.planEnd),
	})

	out = append(out, renderEvent{
		stream: streamOut,
		line:   "",
	})

	return out
}

// collectOpTreeLines builds op-tree lines without the plan rail prefix.
// Each line starts with a leading space (for alignment after plan rail).
func (c *cli) collectOpTreeLines(
	lines *[]string,
	op event.PlannedOp,
	children map[int][]event.PlannedOp,
	prefix []bool,
	isLast bool,
	v signal.Verbosity,
) {
	var b strings.Builder
	b.WriteString(" ") // leading space (plan rail will precede this)

	for i, cont := range prefix {
		var seg string
		if cont {
			seg = c.glyphs.actionRail + " "
		} else {
			seg = c.glyphs.actionIndent
		}

		if i == 0 {
			b.WriteString(c.fmtMsg(colActionRail, seg))
		} else {
			b.WriteString(c.fmtMsg(colOpRail, seg))
		}
	}

	conn := c.glyphs.opBranch
	if isLast {
		conn = c.glyphs.opLast
	}
	b.WriteString(c.fmtMsg(colOpRail, conn))

	line := b.String()
	line += c.fmtMsg(colOpHeader, op.DisplayID)

	if v >= signal.VV && op.Template != nil {
		if text, ok := template.Render(
			op.Template.ID,
			op.Template.Text,
			op.Template.Data,
		); ok {
			line += c.fmtfMsg(colOpDesc, " (%s)", text)
		}
	}

	*lines = append(*lines, line)

	kids := children[op.Index]
	for i, child := range kids {
		last := i == len(kids)-1
		c.collectOpTreeLines(
			lines,
			child,
			children,
			append(prefix, !isLast),
			last,
			v,
		)
	}
}

// Action lifecycle
// ===============================================

func (c *cli) renderActionFinished(e event.ActionEvent) []renderEvent {
	// TODO: make this less convoluted...
	fmtActionSummary := func(s model.ActionSummary) string {
		switch {
		case s.Failed > 0 || s.Aborted > 0:
			return fmt.Sprintf(
				"failed after %d/%d ops (%d failed, %d aborted)",
				s.Succeeded+s.Failed,
				s.Total,
				s.Failed,
				s.Aborted,
			)

		case s.Changed > 0:
			return fmt.Sprintf(
				"%d/%d ops changed",
				s.Changed,
				s.Total,
			)

		case s.WouldChange > 0:
			return fmt.Sprintf(
				"%d/%d ops would change",
				s.WouldChange,
				s.Total,
			)

		default:
			return "up-to-date"
		}
	}

	d := *e.Detail
	s := d.Summary
	st := c.ensureActionFromStep(e.Step)

	var (
		color ansi.ANSI
		glyph string
	)

	switch {
	case s.Failed > 0 || s.Aborted > 0:
		color = colActionFinishedFailed
		glyph = c.glyphs.fatal

	case s.Changed > 0 || s.WouldChange > 0:
		color = colActionFinishedChanged
		glyph = c.glyphs.change

	default:
		color = colActionFinishedUnchanged
		glyph = c.glyphs.ok
	}

	line := c.fmtfMsg(
		color,
		"[%s]%s",
		st.id,
		glyphr(glyph),
	)
	if e.Step.StepDesc != "" {
		line += c.fmtfMsg(
			color,
			" %s —",
			e.Step.StepDesc,
		)
	}
	line += c.fmtfMsg(
		color,
		" %s (%s)",
		fmtActionSummary(s),
		d.Duration,
	)

	return []renderEvent{{
		stream: streamOut,
		line:   line,
	}}
}

// Op lifecycle
// ===============================================

func (c *cli) renderOpChecked(e event.OpEvent) []renderEvent {
	d := *e.CheckDetail
	st := c.ensureActionFromStep(e.Step)

	switch d.Result {
	case spec.CheckSatisfied:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				colOpCheckSatisfied,
				"[%s]%s %s - up-to-date",
				st.id, glyphr(c.glyphs.ok), e.DisplayID,
			),
		}}

	case spec.CheckUnsatisfied:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				colOpCheckUnsatisfied,
				"[%s]%s %s - needs change",
				st.id, glyphr(c.glyphs.change), e.DisplayID,
			),
		}}

	case spec.CheckUnknown:
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtfMsg(
				colOpCheckUnknown,
				"[%s]%s %s - unknown: %v",
				st.id, glyphr(c.glyphs.warn), e.DisplayID, d.Err,
			),
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
			line: c.fmtfMsg(
				colOpExecFailed,
				"[%s]%s '%s' failed: %v",
				st.id, glyphr(c.glyphs.fatal), e.DisplayID, d.Err,
			),
		}}

	case d.Changed:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				colOpExecChanged,
				"[%s]%s '%s' changed (%s)",
				st.id, glyphr(c.glyphs.exec), e.DisplayID, d.Duration,
			),
		}}

	default:
		return nil
	}
}

// Diagnostics
// ===============================================

func (c *cli) EmitEngineDiagnostic(e event.EngineDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}

	context := ""
	if e.CfgPath != "" {
		context = fmt.Sprintf(` in file %q`, e.CfgPath)
	}

	c.renderDiagnostic(
		"engine.error",
		context,
		e.Detail.Template,
	)
}

func (c *cli) EmitPlanDiagnostic(e event.PlanDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}

	c.renderDiagnostic(
		"plan.error",
		fmt.Sprintf(
			` in step [%d|%s] '%s'`,
			e.Step.StepIndex, e.Step.StepKind, e.Step.StepDesc,
		),
		e.Detail.Template,
	)
}

func (c *cli) EmitActionDiagnostic(e event.ActionDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}

	c.renderDiagnostic(
		"action.error",
		fmt.Sprintf(
			` in step [%d|%s] '%s'`,
			e.Step.StepIndex, e.Step.StepKind, e.Step.StepDesc,
		),
		e.Detail.Template,
	)
}

func (c *cli) EmitOpDiagnostic(e event.OpDiagnostic) {
	if !c.shouldRender(e.Chattiness) {
		return
	}

	c.renderDiagnostic(
		"op.error",
		fmt.Sprintf(
			` in op '%s' of step [%d|%s] '%s'`,
			e.DisplayID, e.Step.StepIndex, e.Step.StepKind, e.Step.StepDesc,
		),
		e.Detail.Template,
	)
}

func (c *cli) renderDiagnostic(prefix, msg string, tmpl event.Template) {
	var events []renderEvent
	for _, l := range c.fmtTemplate(
		tmpl,
		prefix,
		msg,
		c.glyphs.error,
		colDiagMsg,
		colDiagHelp,
	) {
		events = append(events, renderEvent{stream: streamErr, line: l})
	}

	c.commitRenderEvents(events)
}

// Helpers
// ===============================================

func (c *cli) ensureActionFromStep(step event.StepDetail) *actionState {
	key := fmt.Sprintf("%s:%d", step.StepKind, step.StepIndex)
	st, _ := c.actions.LoadOrStore(key, &actionState{
		id: key,
	})
	return st.(*actionState)
}

// Message formatting
// ===============================================

func (c *cli) fmtMsg(color ansi.ANSI, msg string) string {
	var buf strings.Builder
	c.fmtMsgTo(&buf, color, msg)
	return buf.String()
}

func (c *cli) fmtfMsg(color ansi.ANSI, format string, args ...any) string {
	var buf strings.Builder
	c.fmtfMsgTo(&buf, color, format, args...)
	return buf.String()
}

func (c *cli) fmtMsgTo(w io.Writer, color ansi.ANSI, msg string) {
	if !c.shouldUseColor() {
		fprint(w, msg)
		return
	}

	fprint(w, color)
	fprint(w, msg)
	fprint(w, ansi.Reset)
}

func (c *cli) fmtfMsgTo(buf *strings.Builder, color ansi.ANSI, format string, args ...any) {
	if !c.shouldUseColor() {
		fprintf(buf, format, args...)
		return
	}

	buf.WriteString(color.String())
	fprintf(buf, format, args...)
	buf.WriteString(ansi.Reset)
}

func (c *cli) fmtTemplate(tmpl event.Template, prefix, msg string, glyph string, txtCol, helpCol ansi.ANSI) []string {
	var buf strings.Builder

	if text, ok := template.Render(tmpl.ID+".Text", tmpl.Text, tmpl.Data); ok {
		c.fmtfMsgTo(
			&buf,
			txtCol,
			"[%s]%s %s%s",
			prefix, glyphr(glyph), text, msg,
		)
	}

	if snippet, ok := c.renderSnippet(tmpl.Source); ok {
		fprintln(&buf)
		fprint(&buf, snippet)
	}

	if hint, ok := template.Render(tmpl.ID+".Hint", tmpl.Hint, tmpl.Data); ok {
		hint = strings.TrimSpace(hint)
		if hint != "" {
			fprint(&buf, "\n    ")
			c.fmtfMsgTo(
				&buf,
				helpCol,
				"%s hint:",
				glyphl(c.glyphs.hint),
			)

			lines := strings.SplitSeq(hint, "\n")
			for l := range lines {
				fprint(&buf, "\n    ")
				c.fmtfMsgTo(
					&buf,
					helpCol,
					"     %s",
					l,
				)
			}
		}
	}

	if help, ok := template.Render(tmpl.ID+".Help", tmpl.Help, tmpl.Data); ok {
		help = strings.TrimSpace(help)
		if help != "" {
			fprint(&buf, "\n    ")
			c.fmtfMsgTo(
				&buf,
				helpCol,
				"%s help:",
				glyphl(c.glyphs.help),
			)

			lines := strings.SplitSeq(help, "\n")
			for l := range lines {
				fprint(&buf, "\n    ")
				c.fmtfMsgTo(
					&buf,
					helpCol,
					"     %s",
					l,
				)
			}
		}
	}

	return strings.Split(strings.TrimSpace(buf.String()), "\n")
}

func (c *cli) renderSnippet(src *spec.SourceSpan) (string, bool) {
	if src == nil || c.store == nil {
		return "", false
	}

	v := c.loadSourceLine(src)

	var b strings.Builder
	c.renderSourceHeader(&b, v)
	fprintln(&b)
	c.renderSourceBody(&b, v)
	return b.String(), true
}

func (c *cli) loadSourceLine(src *spec.SourceSpan) sourceLine {
	text, ok := c.store.Line(src.Filename, src.StartLine)
	endCol := src.EndCol
	if src.StartLine < src.EndLine {
		// underline full line
		endCol = len(text) + 1
	}
	return sourceLine{
		filename: src.Filename,
		line:     src.StartLine,
		startCol: src.StartCol,
		endCol:   endCol,
		text:     text,
		ok:       ok,
	}
}

func (c *cli) renderSourceHeader(w io.Writer, v sourceLine) {
	fprintf(
		w,
		"  --> %s:%d:%d",
		v.filename,
		v.line,
		v.startCol,
	)
}

func (c *cli) renderSourceBody(w io.Writer, v sourceLine) {
	gutter := c.fmtfMsg(colSourceGutter, "|")

	if !v.ok {
		fprintf(w, "   %s <source unavailable>", gutter)
		return
	}

	lineNo := strconv.Itoa(v.line)
	pad := strings.Repeat(" ", len(lineNo))

	// empty gutter line
	fprintf(w, "  %s %s\n", pad, gutter)

	// source line
	fprintf(w, "  %s%s%s %s %s\n", colSourceGutter, lineNo, ansi.Reset, gutter, v.text)

	// caret line
	if v.startCol > 0 {
		fprintf(
			w,
			"  %s %s %s",
			pad,
			gutter,
			caretPadding(v.text, v.startCol),
		)
		c.fmtMsgTo(w, colSourceCaret, underlineRange(v.startCol, v.endCol))
	}
}

func caretPadding(line string, col int) string {
	if col <= 1 {
		return ""
	}

	var b strings.Builder
	i := 1

	for _, r := range line {
		if i >= col {
			break
		}

		switch r {
		case '\t':
			b.WriteRune('\t') // preserve tab exactly
		default:
			// replace any other rune with a single space
			// (including wide Unicode)
			b.WriteRune(' ')
		}

		i++
	}

	return b.String()
}

func underlineRange(start, end int) string {
	if end <= start {
		return "^"
	}
	return strings.Repeat("~", end-start)
}

func (c *cli) shouldUseColor() bool {
	switch c.opts.ColorMode {
	case signal.ColorAlways:
		return true
	case signal.ColorNever:
		return false
	case signal.ColorAuto:
		return c.isTTY
	default:
		return false
	}
}

func glyphr(g string) string {
	return " " + g
}

func glyphl(g string) string {
	return g + " "
}

// Renderer
// ===============================================

type stream uint8

const (
	streamOut stream = iota
	streamErr
)

type (
	renderEvent struct {
		line   string
		stream stream
	}
	renderer struct {
		out io.Writer
		err io.Writer

		ch   chan renderEvent
		done chan struct{}
	}
)

func newRenderer(out, err io.Writer) *renderer {
	r := &renderer{
		out:  out,
		err:  err,
		ch:   make(chan renderEvent, 256),
		done: make(chan struct{}),
	}

	// render loop
	go func() {
		for e := range r.ch {
			w := r.out
			if e.stream == streamErr {
				w = r.err
			}
			fprintln(w, e.line)
		}

		close(r.done)
	}()
	return r
}

func (r *renderer) close() {
	close(r.ch)
	<-r.done
}

func (r *renderer) emitEvents(events []renderEvent) {
	for _, e := range events {
		select {
		case r.ch <- e:
		case <-r.done:
			// renderer is shutting down, drop message
		}
	}
}

func fprintln(w io.Writer, args ...any)               { _, _ = fmt.Fprintln(w, args...) }
func fprint(w io.Writer, args ...any)                 { _, _ = fmt.Fprint(w, args...) }
func fprintf(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
