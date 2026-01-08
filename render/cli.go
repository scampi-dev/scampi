package render

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"
	"godoit.dev/doit/render/ansi"
	"godoit.dev/doit/signal"
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
	symChange = '󰏫' // nf-md-pencil  U+F0EB
	symOK     = '󰄬' // nf-md-check   U+F12C
	symExec   = '󰒓' // nf-md-cog     U+F355
	symWarn   = '󰀦' // nf-md-alert   U+F02A
	symFail   = '󰅖' // nf-md-close   U+F15A
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

	// signal renderer to exit
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

// Engine lifecycle
// ===============================================

func (c *cli) EngineStart(_ signal.Severity) {
	if c.v() >= signal.VV {
		c.outln(
			ansi.Green.Dim,
			"[engine] starting",
		)
	}
}

func (c *cli) EngineFinish(_ signal.Severity, rs RunSummary, dur time.Duration) {
	color := ansi.Green.Reg
	if rs.FailedCount > 0 {
		color = ansi.Red.Reg
	} else if rs.ChangedCount > 0 {
		color = ansi.Yellow.Reg
	}

	c.outln(
		color,
		"[engine] finished (%d change%s, %d failure%s, %d unit%s, %s)",
		rs.ChangedCount, s(rs.ChangedCount),
		rs.FailedCount, s(rs.FailedCount),
		rs.TotalCount, s(rs.TotalCount),
		dur,
	)

	c.render.stop()
}

// Planning lifecycle
// ===============================================

func (c *cli) PlanStart(_ signal.Severity) {
	if c.v() >= signal.VV {
		c.outln(
			ansi.Blue.Reg,
			"[plan] starting",
		)
	}
}

func (c *cli) UnitPlanned(_ signal.Severity, index int, name, kind string) {
	if c.v() >= signal.VVV {
		c.outln(
			ansi.BrightBlack.Dim,
			"[plan.unit] #%d %s '%s'",
			index, kind, name,
		)
	}
}

func (c *cli) PlanFinish(_ signal.Severity, unitCount int, dur time.Duration) {
	if c.v() >= signal.VV {
		c.outln(
			ansi.Blue.Dim,
			"[plan] finished: %d unit%s planned (%s)",
			unitCount, s(unitCount), dur,
		)
	}
}

// Action lifecycle
// ===============================================

func (c *cli) ActionStart(_ signal.Severity, name string) {
	_ = c.ensureAction(name)
}

func (c *cli) ActionFinish(_ signal.Severity, name string, changed bool, dur time.Duration) {
	st := c.ensureAction(name)
	st.finished = true

	if changed {
		c.outln(
			ansi.Yellow.Reg,
			"[%s]%s '%s' changed (%s)",
			st.id, c.glyph(symChange), name, dur,
		)
		return
	}

	if c.v() >= signal.V {
		c.outln(
			ansi.Green.Dim,
			"[%s]%s '%s' up-to-date",
			st.id, c.glyph(symOK), name,
		)
	}
}

func (c *cli) ActionError(_ signal.Severity, name string, err error) {
	st := c.ensureAction(name)

	c.errln(
		ansi.Red.Reg,
		"[%s]%s '%s' failed: %v",
		st.id, c.glyph(symFail), name, err,
	)
}

// OpCheck lifecycle
// ===============================================

func (c *cli) OpCheckStart(_ signal.Severity, _, _ string) {
	// intentionally silent
}

func (c *cli) OpCheckUnsatisfied(_ signal.Severity, action, op string) {
	if c.v() < signal.V {
		return
	}

	st := c.ensureAction(action)
	c.outln(
		ansi.BrightBlack.Dim,
		"[%s]%s '%s' needs change",
		st.id, c.glyph(symChange), op,
	)
}

func (c *cli) OpCheckSatisfied(_ signal.Severity, action, op string) {
	if c.v() < signal.VVV {
		return
	}

	st := c.ensureAction(action)
	c.outln(
		ansi.BrightBlack.Dim,
		"[%s]%s '%s' up-to-date",
		st.id, c.glyph(symOK), op,
	)
}

func (c *cli) OpCheckUnknown(_ signal.Severity, action, op string, err error) {
	st := c.ensureAction(action)
	c.errln(
		ansi.Yellow.Reg,
		"[%s]%s check %s unknown: %v",
		st.id, c.glyph(symWarn), op, err,
	)
}

// OpExecute lifecycle
// ===============================================

func (c *cli) OpExecuteStart(_ signal.Severity, _, _ string) {
	// intentionally silent
}

func (c *cli) OpExecuteFinish(_ signal.Severity, action, op string, changed bool, dur time.Duration) {
	if !changed || c.v() < signal.VV {
		return
	}

	st := c.ensureAction(action)
	c.outln(
		ansi.BrightBlack.Reg,
		"[%s]%s '%s' changed (%s)",
		st.id, c.glyph(symExec), op, dur,
	)
}

func (c *cli) OpExecuteError(_ signal.Severity, action, op string, err error) {
	st := c.ensureAction(action)
	c.errln(
		ansi.Red.Reg,
		"[%s]%s '%s' failed: %v",
		st.id, c.glyph(symFail), op, err,
	)
}

// Errors
// ===============================================

func (c *cli) UserError(_ signal.Severity, msg Message) {
	c.errln(
		ansi.Red.Reg,
		"[error]%s %s",
		c.glyph(symFail), renderMessageCompat(msg),
	)
}

func (c *cli) InternalError(_ signal.Severity, msg Message) {
	c.errln(
		ansi.BrightRed.Bold,
		"[fatal]%s %s",
		c.glyph(symFail), renderMessageCompat(msg),
	)
}

func renderMessageCompat(msg Message) string {
	// TEMPORARY: until i18n / proper rendering exists
	if len(msg.Args) == 0 {
		return msg.Key
	}
	return fmt.Sprintf("%s %v", msg.Key, msg.Args)
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

func (c *cli) glyph(g rune) string {
	return " " + string(g)
}
