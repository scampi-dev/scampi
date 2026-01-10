package render

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"

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
	}
	actionState struct {
		id       string
		finished bool
	}
	sourceLine struct {
		filename string
		line     int
		column   int
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

func NewCLI(opts CLIOptions, store *spec.SourceStore) Displayer {
	return &cli{
		opts:  opts,
		store: store,
		render: newRenderer(
			os.Stdout,
			os.Stderr,
			term.IsTerminal(os.Stdout.Fd()),
		),
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
		line: c.fmtMsg(
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
			line: c.fmtMsg(
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
		line: c.fmtMsg(
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
		line: c.fmtMsg(
			ansi.Blue.Reg,
			"[plan] started",
		),
	}}
}

func (c *cli) renderPlanFinished(e event.Event) []renderEvent {
	d := e.Detail.(event.PlanDetail)

	var events []renderEvent

	ttlUnits := d.SuccessfulUnits + d.FailedUnits
	if d.FailedUnits > 0 {
		events = append(events, renderEvent{
			stream: streamOut,
			line: c.fmtMsg(
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
			line: c.fmtMsg(
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
		line: c.fmtMsg(
			ansi.BrightBlack.Dim,
			"[plan.unit]%s #%d %s '%s'",
			glyphr(symOK),
			e.Subject.Index,
			e.Subject.Kind,
			e.Subject.Name,
		),
	}}
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
			line: c.fmtMsg(
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
			line: c.fmtMsg(
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
			line: c.fmtMsg(
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
			line: c.fmtMsg(
				ansi.BrightBlack.Dim,
				"[%s]%s '%s' up-to-date",
				st.id, glyphr(symOK), e.Subject.Op,
			),
		}}

	case spec.CheckUnsatisfied:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtMsg(
				ansi.BrightBlack.Dim,
				"[%s]%s '%s' needs change",
				st.id, glyphr(symChange), e.Subject.Op,
			),
		}}

	case spec.CheckUnknown:
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtMsg(
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
			line: c.fmtMsg(
				ansi.Red.Reg,
				"[%s]%s '%s' failed: %v",
				st.id, glyphr(symFatal), e.Subject.Op, d.Err,
			),
		}}

	case d.Changed:
		return []renderEvent{{
			stream: streamOut,
			line: c.fmtMsg(
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
	case event.ScopePlan:
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtTemplate(
				d.Template,
				"plan.error",
				fmt.Sprintf(` in unit [%d|%s] '%s'`, sub.Index, sub.Kind, sub.Name),
				symErr,
				ansi.Red.Reg,
				ansi.Cyan.Reg,
			),
		}}

	default:
		return []renderEvent{{
			stream: streamErr,
			line: c.fmtTemplate(
				d.Template,
				fmt.Sprintf("%s.error", e.Scope),
				fmt.Sprintf("\n    -- DEFAULT SCOPE_BRANCH PROBABLY BUG --\n%#v\n\n", e),
				symErr,
				ansi.Red.Reg,
				ansi.Cyan.Reg,
			),
		}}
	}
}

func (c *cli) renderUnknownEvent(e event.Event) []renderEvent {
	return []renderEvent{{
		stream: streamErr,
		line: c.fmtMsg(
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

func (c *cli) fmtMsg(color ansi.Code, format string, args ...any) string {
	var buf strings.Builder
	c.fmtMsgTo(&buf, color, format, args...)
	return buf.String()
}

func (c *cli) fmtMsgTo(buf *strings.Builder, color ansi.Code, format string, args ...any) {
	if !c.shouldUseColor() {
		fprintf(buf, format, args...)
		return
	}

	buf.WriteString(string(color))
	fprintf(buf, format, args...)
	buf.WriteString(string(ansi.Reset))
}

func (c *cli) fmtTemplate(tmpl event.Template, prefix, msg string, glyph rune, txtCol, helpCol ansi.Code) string {
	// tmplText := template.Render(tmpl.ID+".Text", tmpl.Text, tmpl.Data)
	// tmplHint := template.Render(tmpl.ID+".Hint", tmpl.Hint, tmpl.Data)
	// tmplHelp := template.Render(tmpl.ID+".Help", tmpl.Help, tmpl.Data)

	var buf strings.Builder

	if text, ok := template.Render(tmpl.ID+".Text", tmpl.Text, tmpl.Data); ok {
		c.fmtMsgTo(
			&buf,
			txtCol,
			"[%s]%s %s%s",
			prefix, glyphr(glyph), text, msg,
		)
	}

	if snippet, ok := c.renderSnippet(tmpl.Source); ok {
		buf.WriteString("\n")
		buf.WriteString(snippet)
	}

	if hint, ok := template.Render(tmpl.ID+".Hint", tmpl.Hint, tmpl.Data); ok {
		buf.WriteString("\n    ")
		c.fmtMsgTo(
			&buf,
			helpCol,
			"%s hint: %s",
			glyphl(symHint), hint,
		)
	}

	if help, ok := template.Render(tmpl.ID+".Help", tmpl.Help, tmpl.Data); ok {
		buf.WriteString("\n    ")
		c.fmtMsgTo(
			&buf,
			helpCol,
			"%s help: %s",
			glyphl(symHelp), help,
		)
	}

	return buf.String()
}

func (c *cli) renderSnippet(src *spec.SourceSpan) (string, bool) {
	if src == nil {
		return "", false
	}

	v := c.loadSourceLine(src)

	var b strings.Builder
	b.WriteString(c.renderSourceHeader(v))
	b.WriteString("\n")
	b.WriteString(c.renderSourceBody(v))

	return b.String(), true
}

func (c *cli) loadSourceLine(src *spec.SourceSpan) sourceLine {
	text, ok := c.store.Line(src.Filename, src.Line)
	return sourceLine{
		filename: src.Filename,
		line:     src.Line,
		column:   src.Column,
		text:     text,
		ok:       ok,
	}
}

func (c *cli) renderSourceHeader(v sourceLine) string {
	return fmt.Sprintf(
		"  --> %s:%d:%d",
		v.filename,
		v.line,
		v.column,
	)
}

func (c *cli) renderSourceBody(v sourceLine) string {
	if !v.ok {
		return "   | <source unavailable>"
	}

	lineNo := strconv.Itoa(v.line)
	pad := strings.Repeat(" ", len(lineNo))

	var b strings.Builder

	// empty gutter line
	fmt.Fprintf(&b, "  %s |\n", pad)

	// source line
	fmt.Fprintf(&b, "  %s | %s\n", lineNo, v.text)

	// caret line
	if v.column > 0 {
		fmt.Fprintf(
			&b,
			"  %s | %s^\n",
			pad,
			caretPadding(v.text, v.column),
		)
	}

	return strings.TrimRightFunc(b.String(), unicode.IsSpace)
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

func visualColumn(line string, srcCol int) int {
	const tabWidth = 8

	col := 0

	for i, r := range line {
		if i >= srcCol {
			break
		}

		switch r {
		case '\t':
			// advance to next tab stop
			col += tabWidth - (col % tabWidth)
		default:
			col++
		}
	}

	return col
}

func (c *cli) shouldUseColor() bool {
	switch c.opts.ColorMode {
	case signal.ColorAlways:
		return true
	case signal.ColorNever:
		return false
	case signal.ColorAuto:
		return c.render.isTTY
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
		out   io.Writer
		err   io.Writer
		isTTY bool

		ch   chan renderEvent
		done chan struct{}
	}
)

func newRenderer(out, err io.Writer, isTTY bool) *renderer {
	r := &renderer{
		out:   out,
		err:   err,
		isTTY: isTTY,
		ch:    make(chan renderEvent, 256),
		done:  make(chan struct{}),
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
func fprintf(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
