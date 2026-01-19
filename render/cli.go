package render

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/term"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/model"
	"godoit.dev/doit/render/ansi"
	"godoit.dev/doit/render/template"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/util"
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
	}
)

const (
	minWidePlanCols = 70 // below this, fancy, wide plan rendering adds more noise than clarity
)

var (
	colPlan       = ansi.Magenta
	colPlanHeader = colPlan.Bold
	colPlanRail   = colPlan.Dim

	colAction       = ansi.Cyan
	colActionHeader = colAction.Bold
	colActionRail   = colAction.Reg
	colActionOps    = colAction.Dim

	colOp       = ansi.BrightBlack
	colOpHeader = colOp.Reg
	colOpRail   = colOp.Dim
	colOpDesc   = colOp.Dim
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

func (c *cli) Emit(e event.Event) {
	shouldRender := func() bool {
		v := c.opts.Verbosity

		switch e.Chattiness {
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

	if !shouldRender() {
		return
	}

	events := c.toRenderEvents(e)
	// fit lines before committing to draw loop
	for i := range events {
		if strings.ContainsAny(events[i].line, "\n\r") {
			panic("util.BUG: renderEvent.line must neither contain '\\n' nor '\\r'")
		}
		events[i].line = fitLine(events[i].line, c.width)
		if c.shouldUseColor() {
			events[i].line += ansi.Reset
		}
	}
	c.render.emitEvents(events)
}

func (c *cli) Close() {
	c.render.close()
}

func (c *cli) toRenderEvents(e event.Event) []renderEvent {
	switch e.Kind {

	// Engine lifecycle
	// ===============================================
	case event.EngineStarted:
		return c.renderEngineStarted(e)
	case event.EngineFinished:
		return c.renderEngineFinished(e)

	// Plan lifecycle
	// ===============================================
	case event.PlanStarted:
		return c.renderPlanStarted(e)
	case event.PlanFinished:
		return c.renderPlanFinished(e)
	case event.UnitPlanned:
		return c.renderUnitPlanned(e)
	case event.PlanProduced:
		return c.renderPlan(e)

	// Action lifecycle
	// ===============================================
	case event.ActionStarted:
		return nil // intentionally ignored
	case event.ActionFinished:
		return c.renderActionFinished(e)

	// Op lifecycle
	// ===============================================
	case event.OpCheckStarted:
		return nil // intentionally ignored
	case event.OpChecked:
		return c.renderOpChecked(e)
	case event.OpExecuteStarted:
		return nil // intentionally ignored
	case event.OpExecuted:
		return c.renderOpExecuted(e)

	// Diagnostics
	// ===============================================
	case event.DiagnosticRaised:
		return c.renderDiagnosticRaised(e)

	default:
		return c.renderUnknownEvent(e)
	}
}

// Engine lifecycle
// ===============================================

func (c *cli) renderEngineStarted(_ event.Event) []renderEvent {
	return []renderEvent{{
		stream: streamOut,
		line: c.fmtfMsg(
			ansi.Green.Dim,
			"[engine] started",
		),
	}}
}

func (c *cli) renderEngineFinished(e event.Event) []renderEvent {
	d := e.Detail.(event.EngineDetail)

	if d.Err != nil {
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtfMsg(
				ansi.BrightRed.Bold,
				"[engine]%s failed: %v",
				glyphr(c.glyphs.fatal),
				d.Err,
			),
		}}
	}

	color := ansi.Green.Reg
	if d.FailedCount > 0 {
		color = ansi.Red.Reg
	} else if d.ChangedCount > 0 {
		color = ansi.Yellow.Reg
	}

	return []renderEvent{{
		stream: streamOut,
		line: c.fmtfMsg(
			color,
			"[engine] finished (%d change%s, %d failure%s, %d unit%s, %s)",
			d.ChangedCount, s(d.ChangedCount),
			d.FailedCount, s(d.FailedCount),
			d.TotalCount, s(d.TotalCount),
			d.Duration,
		),
	}}
}

// Plan lifecycle
// ===============================================

func (c *cli) renderPlanStarted(_ event.Event) []renderEvent {
	return []renderEvent{{
		stream: streamOut,
		line: c.fmtfMsg(
			ansi.Blue.Reg,
			"[plan] started",
		),
	}}
}

func (c *cli) renderPlanFinished(e event.Event) []renderEvent {
	d := e.Detail.(event.PlanFinishedDetail)

	var events []renderEvent

	ttlUnits := d.SuccessfulUnits + d.FailedUnits
	if d.FailedUnits > 0 {
		events = append(events, renderEvent{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.Red.Reg,
				"[plan]%s aborted: %d/%d unit%s planned, %d/%d unit%s failed (%s)",
				glyphr(c.glyphs.fatal),
				d.SuccessfulUnits,
				ttlUnits,
				s(d.SuccessfulUnits),
				d.FailedUnits,
				ttlUnits,
				s(d.FailedUnits),
				d.Duration,
			),
		})
	} else {
		events = append(events, renderEvent{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.Blue.Dim,
				"[plan] finished: %d unit%s planned (%s)",
				d.SuccessfulUnits,
				s(d.SuccessfulUnits),
				d.Duration,
			),
		})
	}

	return events
}

func (c *cli) renderUnitPlanned(e event.Event) []renderEvent {
	return []renderEvent{{
		stream: streamOut,
		line: c.fmtfMsg(
			ansi.BrightBlack.Dim,
			"[plan.unit]%s #%d %s '%s'",
			glyphr(c.glyphs.ok),
			e.Subject.Index,
			e.Subject.Kind,
			e.Subject.Name,
		),
	}}
}

// renderPlan invariant:
// The plan is represented as a single continuous vertical rail.
// Action rails are nested inside the plan rail.
// Ops never touch the plan rail directly.
func (c *cli) renderPlan(e event.Event) []renderEvent {
	d := e.Detail.(event.PlanDetail)

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

	hdr := c.fmtMsg(colPlanRail, c.glyphs.planStart+" ") +
		c.fmtMsg(colPlanHeader, "execution plan")
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
	for _, act := range dag.Actions {
		if act.Index > maxIndex {
			maxIndex = act.Index
		}
	}

	indexWidth := digits10(maxIndex)

	for _, act := range dag.Actions {
		kind := ""
		if act.Kind != "" {
			kind = fmt.Sprintf(" %s %s", act.Kind, c.glyphs.actionKindSep)
		}

		if v == signal.Quiet {
			nOps := 0
			for _, l := range act.Layers {
				nOps += len(l)
			}

			line := c.fmtMsg(colActionRail, " "+c.glyphs.actionStartCollapsed) +
				c.fmtfMsg(
					colActionHeader,
					" [%*d]%s %s",
					indexWidth,
					act.Index,
					kind,
					act.Name,
				)

			var opLine string
			switch nOps {
			case 0:
				// this would be very odd tbh, but ¯\_(ツ)_/¯
				opLine = " (noop)"
			case 1:
				opLine = " (1 op)"
			default:
				opLine = fmt.Sprintf(" (%d ops)", nOps)
			}

			line += c.fmtMsg(
				colActionOps,
				opLine,
			)

			line = c.fmtMsg(colPlanRail, c.glyphs.planRail) + line

			out = append(out, renderEvent{
				stream: streamOut,
				line:   line,
			})
			continue
		}

		gutter := c.glyphs.actionStart
		if len(act.Layers) == 0 {
			gutter = c.glyphs.actionStartNoOp
		}

		{
			line := c.fmtMsg(colPlanRail, c.glyphs.planRail+" ") +
				c.fmtMsg(colActionRail, gutter) +
				c.fmtfMsg(
					colActionHeader,
					" [%*d]%s %s",
					indexWidth,
					act.Index,
					kind,
					act.Name,
				)
			out = append(out, renderEvent{
				stream: streamOut,
				line:   line,
			})
		}

		ops := flattenLayers(act.Layers)

		children := buildDepTree(ops)
		roots := findRoots(ops)

		for i, root := range roots {
			c.renderOpTree(
				&out,
				root,
				children,
				[]bool{true},
				i == len(roots)-1,
				v,
			)
		}

		line := c.fmtMsg(colPlanRail, c.glyphs.planRail+" ") +
			c.fmtMsg(colActionRail, c.glyphs.actionEnd)

		out = append(out, renderEvent{
			stream: streamOut,
			line:   line,
		})
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

func (c *cli) renderOpTree(
	out *[]renderEvent,
	op event.PlannedOp,
	children map[int][]event.PlannedOp,
	prefix []bool,
	isLast bool,
	v signal.Verbosity,
) {
	// Build gutter
	var b strings.Builder

	for i, cont := range prefix {
		var seg string
		if cont {
			seg = c.glyphs.actionRail + " "
		} else {
			seg = c.glyphs.actionIndent
		}

		if i == 0 {
			// Action-level gutter
			b.WriteString(c.fmtMsg(colActionRail, seg))
		} else {
			// Op-level gutter
			b.WriteString(c.fmtMsg(colOpRail, seg))
		}
	}

	// Connector for this node
	conn := c.glyphs.opBranch
	if isLast {
		conn = c.glyphs.opLast
	}
	b.WriteString(c.fmtMsg(colOpRail, conn))

	line := b.String()
	line += c.fmtMsg(colOpHeader, op.Name)

	if v >= signal.VV && op.Template != nil {
		if text, ok := template.Render(
			op.Template.ID,
			op.Template.Text,
			op.Template.Data,
		); ok {
			line += c.fmtfMsg(colOpDesc, " (%s)", text)
		}
	}

	line = c.fmtMsg(colPlanRail, c.glyphs.planRail+" ") + line
	*out = append(*out, renderEvent{
		stream: streamOut,
		line:   line,
	})

	// Recurse into children
	kids := children[op.Index]
	for i, child := range kids {
		last := i == len(kids)-1
		c.renderOpTree(
			out,
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

func (c *cli) renderActionFinished(e event.Event) []renderEvent {
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

		default:
			return "up-to-date"
		}
	}

	d := e.Detail.(event.ActionDetail)
	s := d.Summary
	st := c.ensureAction(e.Subject.Action)

	var (
		color ansi.Code
		glyph string
	)

	switch {
	case s.Failed > 0 || s.Aborted > 0:
		color = ansi.Red.Reg
		glyph = c.glyphs.fatal

	case s.Changed > 0:
		color = ansi.Yellow.Reg
		glyph = c.glyphs.change

	default:
		color = ansi.Green.Dim
		glyph = c.glyphs.ok
	}

	return []renderEvent{{
		stream: streamOut,
		line: c.fmtfMsg(
			color,
			"[%s]%s %s — %s (%s)",
			st.id,
			glyphr(glyph),
			e.Subject.Action,
			fmtActionSummary(s),
			d.Duration,
		),
	}}
}

// Op lifecycle
// ===============================================

func (c *cli) renderOpChecked(e event.Event) []renderEvent {
	d := e.Detail.(event.OpCheckDetail)
	st := c.ensureAction(e.Subject.Action)

	switch d.Result {
	case spec.CheckSatisfied:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.BrightBlack.Dim,
				"[%s]%s '%s' up-to-date",
				st.id, glyphr(c.glyphs.ok), e.Subject.Op,
			),
		}}

	case spec.CheckUnsatisfied:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.BrightBlack.Dim,
				"[%s]%s '%s' needs change",
				st.id, glyphr(c.glyphs.change), e.Subject.Op,
			),
		}}

	case spec.CheckUnknown:
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtfMsg(
				ansi.Yellow.Reg,
				"[%s]%s check %s unknown: %v",
				st.id, glyphr(c.glyphs.warn), e.Subject.Op, d.Err,
			),
		}}
	}

	return nil
}

func (c *cli) renderOpExecuted(e event.Event) []renderEvent {
	d := e.Detail.(event.OpExecuteDetail)
	st := c.ensureAction(e.Subject.Action)

	switch {
	case d.Err != nil:
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtfMsg(
				ansi.Red.Reg,
				"[%s]%s '%s' failed: %v",
				st.id, glyphr(c.glyphs.fatal), e.Subject.Op, d.Err,
			),
		}}

	case d.Changed:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.BrightBlack.Reg,
				"[%s]%s '%s' changed (%s)",
				st.id, glyphr(c.glyphs.exec), e.Subject.Op, d.Duration,
			),
		}}

	default:
		return nil
	}
}

// Diagnostics
// ===============================================

func (c *cli) renderDiagnosticRaised(e event.Event) []renderEvent {
	d := e.Detail.(event.DiagnosticDetail)
	sub := e.Subject

	switch e.Scope {
	case event.ScopeEngine:
		var events []renderEvent
		for _, l := range c.fmtTemplate(
			d.Template,
			"engine.error",
			fmt.Sprintf(` in file %q`, sub.CfgPath),
			c.glyphs.error,
			ansi.Red.Reg,
			ansi.Cyan.Reg,
		) {
			events = append(events, renderEvent{stream: streamErr, line: l})
		}
		return events

	case event.ScopePlan:
		var events []renderEvent
		for _, l := range c.fmtTemplate(
			d.Template,
			"plan.error",
			fmt.Sprintf(` in unit [%d|%s] '%s'`, sub.Index, sub.Kind, sub.Name),
			c.glyphs.error,
			ansi.Red.Reg,
			ansi.Cyan.Reg,
		) {
			events = append(events, renderEvent{stream: streamErr, line: l})
		}
		return events

	case event.ScopeOp:
		var events []renderEvent
		for _, l := range c.fmtTemplate(
			d.Template,
			"plan.error",
			fmt.Sprintf(` in unit [%d|%s] '%s'`, sub.Index, sub.Kind, sub.Name),
			c.glyphs.error,
			ansi.Red.Reg,
			ansi.Cyan.Reg,
		) {
			events = append(events, renderEvent{stream: streamErr, line: l})
		}
		return events

	default:
		panic(util.BUG(
			"renderer encountered unsupported event scope %q for event %#v",
			e.Scope,
			e,
		))
	}
}

func (c *cli) renderUnknownEvent(e event.Event) []renderEvent {
	return []renderEvent{{
		stream: streamErr,
		line: c.fmtfMsg(
			ansi.Red.Reg,
			"[unknown]%s unknown event kind '%s': %+v",
			glyphr(c.glyphs.warn), e.Kind, e,
		),
	}}
}

// Helpers
// ===============================================

func (c *cli) ensureAction(name string) *actionState {
	st, _ := c.actions.LoadOrStore(name, &actionState{
		id: makeID(name),
	})
	return st.(*actionState)
}

// makeID produces a short, stable, human-friendly identifier
func makeID(name string) string {
	base := strings.ToLower(name)
	base = strings.ReplaceAll(base, `"`, "")
	base = strings.Fields(base)[0]

	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	suffix := h.Sum32() & 0xff

	return fmt.Sprintf("%s:%02x", base, suffix)
}

// Message formatting
// ===============================================

func (c *cli) fmtMsg(color ansi.Code, msg string) string {
	var buf strings.Builder
	c.fmtMsgTo(&buf, color, msg)
	return buf.String()
}

func (c *cli) fmtfMsg(color ansi.Code, format string, args ...any) string {
	var buf strings.Builder
	c.fmtfMsgTo(&buf, color, format, args...)
	return buf.String()
}

func (c *cli) fmtMsgTo(w io.Writer, color ansi.Code, msg string) {
	if !c.shouldUseColor() {
		fprint(w, msg)
		return
	}

	fprint(w, color)
	fprint(w, msg)
	fprint(w, ansi.Reset)
}

func (c *cli) fmtfMsgTo(buf *strings.Builder, color ansi.Code, format string, args ...any) {
	if !c.shouldUseColor() {
		fprintf(buf, format, args...)
		return
	}

	buf.WriteString(string(color))
	fprintf(buf, format, args...)
	buf.WriteString(string(ansi.Reset))
}

func (c *cli) fmtTemplate(tmpl event.Template, prefix, msg string, glyph string, txtCol, helpCol ansi.Code) []string {
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
	if src == nil {
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
	text, ok := c.store.Line(src.Filename, src.Line)
	return sourceLine{
		filename: src.Filename,
		line:     src.Line,
		startCol: src.StartCol,
		endCol:   src.EndCol,
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
	gutterCol := ansi.BrightBlack.Reg
	gutter := c.fmtfMsg(gutterCol, "|")

	if !v.ok {
		fprintf(w, "   %s <source unavailable>", gutter)
		return
	}

	lineNo := strconv.Itoa(v.line)
	pad := strings.Repeat(" ", len(lineNo))

	// empty gutter line
	fprintf(w, "  %s %s\n", pad, gutter)

	// source line
	fprintf(w, "  %s%s%s %s %s\n", gutterCol, lineNo, ansi.Reset, gutter, v.text)

	// caret line
	if v.startCol > 0 {
		fprintf(
			w,
			"  %s %s %s",
			pad,
			gutter,
			caretPadding(v.text, v.startCol),
		)
		c.fmtMsgTo(w, ansi.Red.Reg, underlineRange(v.startCol, v.endCol))
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
