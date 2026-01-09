package render

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
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
		actions sync.Map // map[string]*actionState
	}
	actionState struct {
		id       string
		finished bool
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

func NewCLI(opts CLIOptions) Displayer {
	return &cli{
		opts: opts,
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

	for _, p := range d.Problems {
		events = append(events, renderEvent{
			stream: streamErr,
			line: c.fmtMsg(
				ansi.Red.Reg,
				"[plan.error]%s [%d|%s] '%s': %v",
				glyphr(symErr),
				p.Index,
				p.Kind,
				p.Name,
				p.Err,
			),
		})
	}

	nProblems := len(d.Problems)
	if nProblems > 0 {
		events = append(events, renderEvent{
			stream: streamOut,
			line: c.fmtMsg(
				ansi.Red.Reg,
				"[plan]%s aborted: %d unit%s planned, %d unit%s failed (%s)",
				glyphr(symFatal),
				d.UnitCount,
				s(d.UnitCount),
				nProblems,
				s(nProblems),
				d.Duration,
			),
		})
	} else {
		events = append(events, renderEvent{
			stream: streamOut,
			line: c.fmtMsg(
				ansi.Blue.Dim,
				"[plan] finished: %d unit%s planned (%s)",
				d.UnitCount,
				s(d.UnitCount),
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
			"[plan.unit] #%d %s '%s'",
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
	if !c.shouldUseColor() {
		return fmt.Sprintf(format, args...)
	}
	return string(color) + fmt.Sprintf(format, args...) + string(ansi.Reset)
}

func (c *cli) fmtTemplate(tmpl event.Template, prefix, msg string, glyph rune, txtCol, helpCol ansi.Code) string {
	tmplText := template.Render(tmpl.ID+".Text", tmpl.Text, tmpl.Data)
	tmplHint := template.Render(tmpl.ID+".Hint", tmpl.Hint, tmpl.Data)
	tmplHelp := template.Render(tmpl.ID+".Help", tmpl.Help, tmpl.Data)

	text := c.fmtMsg(
		txtCol,
		"[%s]%s %s%s",
		prefix, glyphr(glyph), tmplText, msg,
	)

	var hint string
	var help string
	if tmplHint != "" {
		hint = "\n    " + c.fmtMsg(
			helpCol,
			"%s hint: %s",
			glyphl(symHint), tmplHint,
		)
	}
	if tmplHelp != "" {
		help = "\n    " + c.fmtMsg(
			helpCol,
			"%s help: %s",
			glyphl(symHelp), tmplHelp,
		)
	}

	return text + hint + help
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
			_, _ = fmt.Fprintln(w, e.line)
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
