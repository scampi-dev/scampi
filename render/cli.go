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
	renderEvent struct {
		toErr bool
		line  string
	}
	renderer struct {
		out   io.Writer
		err   io.Writer
		isTTY bool

		ch   chan renderEvent
		done chan struct{}
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

func newRenderer(out, err io.Writer, isTTY bool) *renderer {
	r := &renderer{
		out:   out,
		err:   err,
		isTTY: isTTY,
		ch:    make(chan renderEvent, 256),
		done:  make(chan struct{}),
	}

	go r.loop()
	return r
}

func (r *renderer) stop() {
	close(r.ch)
	<-r.done
}

func (r *renderer) loop() {
	for ev := range r.ch {
		w := r.out
		if ev.toErr {
			w = r.err
		}
		_, _ = fmt.Fprintln(w, ev.line)
	}

	close(r.done)
}

func (r *renderer) emit(toErr bool, line string) {
	select {
	case r.ch <- renderEvent{
		toErr: toErr,
		line:  line,
	}:
	case <-r.done:
		// renderer is shutting down, drop message
	}
}

func (c *cli) Emit(e event.Event) {
	if !c.shouldRender(e) {
		return
	}

	sub := e.Subject

	switch e.Kind {

	// Engine lifecycle
	// ===============================================

	case event.EngineStarted:
		c.outln(
			ansi.Green.Dim,
			"[engine] started (NEW)",
		)

	case event.EngineFinished:
		d := e.Detail.(event.EngineDetail)

		switch {
		case d.Err != nil:
			c.errln(
				ansi.BrightRed.Bold,
				"[engine]%s failed: %v (NEW)",
				c.glyph(symFatal),
				d.Err,
			)

		default:
			color := ansi.Green.Reg

			if d.FailedCount > 0 {
				color = ansi.Red.Reg
			} else if d.ChangedCount > 0 {
				color = ansi.Yellow.Reg
			}

			c.outln(
				color,
				"[engine] finished (%d change%s, %d failure%s, %d unit%s, %s) (NEW)",
				d.ChangedCount, s(d.ChangedCount),
				d.FailedCount, s(d.FailedCount),
				d.TotalCount, s(d.TotalCount),
				d.Duration,
			)
		}

		// Plan lifecycle
		// ===============================================

	case event.PlanStarted:
		c.outln(
			ansi.Blue.Reg,
			"[plan] started (NEW)",
		)

	case event.PlanFinished:
		d := e.Detail.(event.PlanDetail)

		for _, p := range d.Problems {
			c.errln(
				ansi.Red.Reg,
				"[plan]%s [%d|%s] '%s': %v (NEW)",
				c.glyph(symFatal),
				p.Index,
				p.Kind,
				p.Name,
				p.Err,
			)
		}

		c.outln(
			ansi.Blue.Dim,
			"[plan] finished: %d unit%s planned (%s) (NEW)",
			d.UnitCount,
			s(d.UnitCount),
			d.Duration,
		)

	case event.UnitPlanned:
		c.outln(
			ansi.BrightBlack.Dim,
			"[plan.unit] #%d %s '%s' (NEW)",
			e.Subject.Index,
			e.Subject.Kind,
			e.Subject.Name,
		)

	// Action lifecycle
	// ===============================================

	case event.ActionStarted:
		// intentionally ignored for now

	case event.ActionFinished:
		d := e.Detail.(event.ActionDetail)
		st := c.ensureAction(e.Subject.Action)

		switch {
		case d.Err != nil:
			c.errln(
				ansi.Red.Reg,
				"[%s]%s '%s' failed: %v (NEW)",
				st.id,
				c.glyph(symFatal),
				e.Subject.Action,
				d.Err,
			)

		case d.Changed:
			c.outln(
				ansi.Yellow.Reg,
				"[%s]%s '%s' changed (%s) (NEW)",
				st.id,
				c.glyph(symChange),
				e.Subject.Action,
				d.Duration,
			)

		default:
			c.outln(
				ansi.Green.Dim,
				"[%s]%s '%s' up-to-date (NEW)",
				st.id,
				c.glyph(symOK),
				e.Subject.Action,
			)
		}

	// Op lifecycle
	// ===============================================

	case event.OpCheckStarted:
		// intentionally ignored for now

	case event.OpChecked:
		d := e.Detail.(event.OpCheckDetail)
		st := c.ensureAction(sub.Action)

		switch d.Result {
		case spec.CheckSatisfied:
			c.outln(
				ansi.BrightBlack.Dim,
				"[%s]%s '%s' up-to-date (NEW)",
				st.id, c.glyph(symOK), sub.Op,
			)

		case spec.CheckUnsatisfied:
			c.outln(
				ansi.BrightBlack.Dim,
				"[%s]%s '%s' needs change (NEW)",
				st.id, c.glyph(symChange), sub.Op,
			)

		case spec.CheckUnknown:
			c.errln(
				ansi.Yellow.Reg,
				"[%s]%s check %s unknown: %v (NEW)",
				st.id, c.glyph(symWarn), sub.Op, d.Err,
			)
		}
	case event.OpExecuteStarted:
		// intentionally ignored for now

	case event.OpExecuted:
		d := e.Detail.(event.OpExecuteDetail)
		st := c.ensureAction(sub.Action)

		switch {
		case d.Err != nil:
			c.errln(
				ansi.Red.Reg,
				"[%s]%s '%s' failed: %v (NEW)",
				st.id, c.glyph(symFatal), sub.Op, d.Err,
			)

		case d.Changed:
			c.outln(
				ansi.BrightBlack.Reg,
				"[%s]%s '%s' changed (%s) (NEW)",
				st.id, c.glyph(symExec), sub.Op, d.Duration,
			)

		default:
			// no-op execution; intentionally quiet for now
		}

	case event.DiagnosticRaised:
		d := e.Detail.(event.DiagnosticDetail)
		sub := e.Subject

		switch e.Scope {
		// case event.ScopeEngine:
		case event.ScopePlan:
			c.emitTemplate(
				d.Template,
				"plan.error",
				fmt.Sprintf(` in unit [%d|%s] '%s'`, sub.Index, sub.Kind, sub.Name),
				symErr,
				ansi.Red.Reg,
				ansi.Cyan.Reg,
			)
		// case event.ScopeAction:
		// case event.ScopeOp:
		default:
			c.emitTemplate(
				d.Template,
				fmt.Sprintf("%s.error", e.Scope),
				fmt.Sprintf("\n    -- DEFAULT SCOPE_BRANCH PROBABLY BUG --\n%#v\n\n", e),
				symErr,
				ansi.Red.Reg,
				ansi.Cyan.Reg,
			)
		}

	default:
		c.errln(
			ansi.Red.Reg,
			"[unknown]%s unknown event kind '%s': %+v",
			c.glyph(symWarn), e.Kind, e,
		)
	}
}

func (c *cli) shouldRender(e event.Event) bool {
	switch e.Chattiness {
	case event.Yappy:
		return c.v() >= signal.VVV
	case event.Chatty:
		return c.v() >= signal.VV
	case event.Normal:
		return c.v() >= signal.V
	case event.Reserved, event.Subtle:
		return true
	default:
		return true
	}
}

func (c *cli) emitTemplate(tmpl event.Template, prefix, msg string, glyph rune, txtCol, helpCol ansi.Code) {
	tmplText := template.Render(tmpl.ID+".Text", tmpl.Text, tmpl.Data)
	tmplHint := template.Render(tmpl.ID+".Hint", tmpl.Hint, tmpl.Data)
	tmplHelp := template.Render(tmpl.ID+".Help", tmpl.Help, tmpl.Data)

	text := c.paint(
		txtCol,
		"[%s]%s %s%s",
		prefix, c.glyph(glyph), tmplText, msg,
	)

	var hint string
	var help string
	if tmplHint != "" {
		hint = "\n    " + c.paint(
			helpCol,
			"%s hint: %s",
			c.glyphl(symHint), tmplHint,
		)
	}
	if tmplHelp != "" {
		help = "\n    " + c.paint(
			helpCol,
			"%s help: %s",
			c.glyphl(symHelp), tmplHelp,
		)
	}

	c.render.emit(true, text+hint+help)
}

// Helpers
// ===============================================

func (c *cli) v() signal.Verbosity {
	return c.opts.Verbosity
}

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

// Output
// ===============================================

func (c *cli) outln(color ansi.Code, format string, args ...any) {
	c.render.emit(false, c.paint(color, format, args...))
}

func (c *cli) errln(color ansi.Code, format string, args ...any) {
	c.render.emit(true, c.paint(color, format, args...))
}

func (c *cli) paint(color ansi.Code, format string, args ...any) string {
	if !c.shouldUseColor() {
		return fmt.Sprintf(format, args...)
	}
	return string(color) + fmt.Sprintf(format, args...) + string(ansi.Reset)
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

func (c *cli) glyphl(g rune) string {
	return string(g) + " "
}

func (c *cli) glyph(g rune) string {
	return " " + string(g)
}
