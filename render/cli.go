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
	"godoit.dev/doit/render/ansi"
	"godoit.dev/doit/render/template"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type (
	CLIOptions struct {
		ColorMode signal.ColorMode
		Verbosity signal.Verbosity
	}

	cli struct {
		opts    CLIOptions
		render  *renderer
		store   *spec.SourceStore
		actions sync.Map // map[string]*actionState

		// layout
		isTTY bool
		width int
	}
	actionState struct {
		id       string
		finished bool
	}
	sourceLine struct {
		filename string
		line     int
		startCol int
		endCol   int
		text     string
		ok       bool
	}
)

// Nerdfont state glyphs
// ===============================================

const (
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

	symChange = '󰏫'
	symOK     = '󰄬'
	symExec   = '󰐊'
	symWarn   = '󰀦'
	symErr    = '󰅖'
	symFatal  = '󰚌'
	symHint   = '󰌵'
	symHelp   = '󰋖'
)

const (
	minWidePlanCols = 70 // below this, fancy, wide plan rendering adds more noise than clarity
)

func NewCLI(opts CLIOptions, store *spec.SourceStore) Displayer {
	determineWidth := func() int {
		if cols := os.Getenv("COLUMNS"); cols != "" {
			if n, err := strconv.Atoi(cols); err == nil && n > 0 {
				return n
			}
		}

		if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
			return w
		}

		return 0
	}

	return &cli{
		opts:  opts,
		store: store,
		render: newRenderer(
			os.Stdout,
			os.Stderr,
		),
		isTTY: term.IsTerminal(os.Stdout.Fd()),
		width: determineWidth(),
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
			panic("BUG: renderEvent.line must neither contain '\\n' nor '\\r'")
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
				glyphr(symFatal),
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
				glyphr(symFatal),
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
			glyphr(symOK),
			e.Subject.Index,
			e.Subject.Kind,
			e.Subject.Name,
		),
	}}
}

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
		line: c.fmtMsg(
			ansi.Magenta.Bold,
			"---[ PLAN ]---",
		),
	})

	dag := buildPlanDAG(d)

	for _, act := range dag.Actions {
		kind := ""
		if act.Kind != "" {
			kind = fmt.Sprintf(" %s ›", act.Kind)
		}
		gutter := " •"
		// gutter := " ▸"
		if v > signal.Quiet {
			gutter = "┌─"
		}
		out = append(out, renderEvent{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.Cyan.Bold,
				"%s [%d]%s %s",
				gutter,
				act.Index,
				kind,
				act.Name,
			),
		})

		if v == signal.Quiet {
			continue
		}

		ops := flattenLayers(act.Layers)

		// index ops
		index := make(map[int]event.PlannedOp)
		for _, op := range ops {
			index[op.Index] = op
		}

		depthMemo := make(map[int]int)

		for i, op := range ops {
			depth := opDepth(op, index, depthMemo)

			prefix := "└─"
			if i < len(ops)-1 {
				prefix = "├─"
			}

			indent := strings.Repeat("  ", depth)

			line := c.fmtMsg(
				ansi.Cyan.Bold,
				"│",
			)
			line += "  "
			line += c.fmtfMsg(
				ansi.BrightBlack.Bold,
				"%s%s %s",
				indent,
				prefix,
				op.Name,
			)

			if v >= signal.VV && op.Template != nil {
				if text, ok := template.Render(
					op.Template.ID,
					op.Template.Text,
					op.Template.Data,
				); ok {
					line += c.fmtfMsg(
						ansi.BrightBlack.Dim,
						" (%s)",
						text,
					)
				}
			}

			out = append(out, renderEvent{
				stream: streamOut,
				line:   line,
			})
		}

		out = append(out, renderEvent{
			stream: streamOut,
			line:   c.fmtMsg(ansi.Cyan.Bold, "└─>"),
		})
	}

	return out
}

// Action lifecycle
// ===============================================

func (c *cli) renderActionFinished(e event.Event) []renderEvent {
	d := e.Detail.(event.ActionDetail)
	st := c.ensureAction(e.Subject.Action)

	switch {
	case d.Err != nil:
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtfMsg(
				ansi.Red.Reg,
				"[%s]%s '%s' failed: %v",
				st.id,
				glyphr(symFatal),
				e.Subject.Action,
				d.Err,
			),
		}}

	case d.Changed:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.Yellow.Reg,
				"[%s]%s '%s' changed (%s)",
				st.id,
				glyphr(symChange),
				e.Subject.Action,
				d.Duration,
			),
		}}

	default:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.Green.Dim,
				"[%s]%s '%s' up-to-date",
				st.id,
				glyphr(symOK),
				e.Subject.Action,
			),
		}}
	}
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
				st.id, glyphr(symOK), e.Subject.Op,
			),
		}}

	case spec.CheckUnsatisfied:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.BrightBlack.Dim,
				"[%s]%s '%s' needs change",
				st.id, glyphr(symChange), e.Subject.Op,
			),
		}}

	case spec.CheckUnknown:
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtfMsg(
				ansi.Yellow.Reg,
				"[%s]%s check %s unknown: %v",
				st.id, glyphr(symWarn), e.Subject.Op, d.Err,
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
				st.id, glyphr(symFatal), e.Subject.Op, d.Err,
			),
		}}

	case d.Changed:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtfMsg(
				ansi.BrightBlack.Reg,
				"[%s]%s '%s' changed (%s)",
				st.id, glyphr(symExec), e.Subject.Op, d.Duration,
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
			symErr,
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
			symErr,
			ansi.Red.Reg,
			ansi.Cyan.Reg,
		) {
			events = append(events, renderEvent{stream: streamErr, line: l})
		}
		return events

	default:
		panic(fmt.Errorf(
			"BUG: renderer encountered unsupported event scope %q for event %#v",
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
			glyphr(symWarn), e.Kind, e,
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

func (c *cli) fmtTemplate(tmpl event.Template, prefix, msg string, glyph rune, txtCol, helpCol ansi.Code) []string {
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
		fprint(&buf, "\n    ")
		c.fmtfMsgTo(
			&buf,
			helpCol,
			"%s hint: %s",
			glyphl(symHint), hint,
		)
	}

	if help, ok := template.Render(tmpl.ID+".Help", tmpl.Help, tmpl.Data); ok {
		fprint(&buf, "\n    ")
		c.fmtfMsgTo(
			&buf,
			helpCol,
			"%s help: %s",
			glyphl(symHelp), help,
		)
	}

	return strings.Split(buf.String(), "\n")
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
		c.fmtMsgTo(w, ansi.Red.Reg, underlineRange(v.text, v.startCol, v.endCol))
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

func underlineRange(line string, start, end int) string {
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

func glyphr(g rune) string {
	return " " + string(g)
}

func glyphl(g rune) string {
	return string(g) + " "
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
