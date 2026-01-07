package render

import (
	"fmt"
	"io"
	"os"
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
		opts CLIOptions

		mu  sync.Mutex
		out io.Writer
		err io.Writer

		isTTY   bool
		actions sync.Map // map[string]*actionRenderState
	}
	actionRenderState struct {
		mu            sync.Mutex
		headerPrinted bool
		finished      bool
	}
)

func NewCLI(opts CLIOptions) Displayer {
	return &cli{
		opts:  opts,
		out:   os.Stdout,
		err:   os.Stderr,
		isTTY: term.IsTerminal(os.Stdout.Fd()),
	}
}

// Engine lifecycle
// ===============================================

func (c *cli) EngineStart(_ signal.Severity) {
	if c.v() >= signal.VV {
		c.outln(
			ansi.Green.Dim,
			`[engine] starting`,
		)
	}
}

func (c *cli) EngineFinish(_ signal.Severity, changed, units int, dur time.Duration) {
	if changed > 0 {
		c.outln(
			ansi.Yellow.Bold,
			`[engine] finished (%d change%s, %d unit%s, %s)`,
			changed, s(changed), units, s(units), dur,
		)
	} else {
		c.outln(
			ansi.Green.Reg,
			`[engine] finished (no changes, %d unit%s, %s)`,
			units, s(units), dur,
		)
	}
}

// Planning
// ===============================================

func (c *cli) PlanStart(_ signal.Severity) {
	if c.v() >= signal.VV {
		c.outln(
			ansi.Blue.Reg,
			`[plan] start`,
		)
	}
}

func (c *cli) UnitPlanned(_ signal.Severity, index int, name, kind string) {
	if c.v() >= signal.VVV {
		c.outln(
			ansi.BrightBlack.Dim,
			`  [unit] #%d %s (%s)`,
			index, name, kind,
		)
	}
}

func (c *cli) PlanFinish(_ signal.Severity, units int, dur time.Duration) {
	if c.v() >= signal.VV {
		c.outln(
			ansi.Blue.Dim,
			`[plan] %d unit%s (%s)`,
			units, s(units), dur,
		)
	}
}

// Actions
// ===============================================

func (c *cli) ActionStart(_ signal.Severity, name string) {
	_ = c.actionState(name)
}

func (c *cli) ActionFinish(_ signal.Severity, name string, changed bool, dur time.Duration) {
	st := c.actionState(name)

	st.mu.Lock()
	defer st.mu.Unlock()

	st.finished = true

	if changed {
		// Changed actions always print, even at verbosity 0
		c.outln(
			ansi.Yellow.Reg,
			`[action] %s changed (%s)`,
			name, dur,
		)
		return
	}

	// No changes
	if c.v() >= signal.V {
		c.outln(
			ansi.Green.Dim,
			`[action] %s up-to-date`,
			name,
		)
	}
}

func (c *cli) ActionError(_ signal.Severity, name string, err error) {
	c.errln(
		ansi.Red.Bold,
		`[action] %s failed: %v`,
		name, err,
	)
}

// Checks (collapsed semantics)
// ===============================================

func (c *cli) OpCheckStart(_ signal.Severity, action, op string) {
	// silent by design
}

func (c *cli) OpCheckUnsatisfied(_ signal.Severity, action, op string) {
	if c.v() < signal.V {
		return
	}

	c.ensureActionHeader(action)
	c.outln(
		ansi.BrightBlack.Dim,
		`  needs change: %s`,
		op,
	)
}

func (c *cli) OpCheckSatisfied(_ signal.Severity, action, op string) {
	if c.v() < signal.VVV {
		return
	}

	c.ensureActionHeader(action)
	c.outln(
		ansi.BrightBlack.Dim,
		`    ✓ %s`,
		op,
	)
}

func (c *cli) OpCheckUnknown(_ signal.Severity, _, op string, err error) {
	c.errln(
		ansi.Yellow.Bold,
		`  check %s unknown: %v`,
		op, err,
	)
}

// Execution
// ===============================================

func (c *cli) OpExecuteStart(_ signal.Severity, action, op string) {
	// intentionally silent
}

func (c *cli) OpExecuteFinish(_ signal.Severity, action, op string, changed bool, dur time.Duration) {
	if !changed || c.v() < signal.VV {
		return
	}

	c.ensureActionHeader(action)
	c.outln(
		ansi.BrightBlack.Reg,
		`  exec %s changed (%s)`,
		op, dur,
	)
}

func (c *cli) OpExecuteError(_ signal.Severity, action, op string, err error) {
	c.errln(
		ansi.Red.Bold,
		`  exec %s failed: %v`,
		op, err,
	)
}

// User-facing errors
// ===============================================

func (c *cli) UserError(_ signal.Severity, message string) {
	c.errln(
		ansi.Red.Reg,
		`[error] %s`,
		message,
	)
}

// Internal errors
// ===============================================

func (c *cli) InternalError(_ signal.Severity, message string, err error) {
	if err != nil {
		c.errln(
			ansi.BrightRed.Bold,
			`[fatal] %s: %v`,
			message, err,
		)
		return
	}
	c.errln(
		ansi.BrightRed.Bold,
		`[fatal] %s`,
		message,
	)
}

// Internal helpers
// ===============================================

func (c *cli) v() signal.Verbosity {
	return c.opts.Verbosity
}

func (c *cli) actionState(name string) *actionRenderState {
	st, _ := c.actions.LoadOrStore(name, &actionRenderState{})
	return st.(*actionRenderState)
}

func (c *cli) ensureActionHeader(name string) {
	// Headers only exist at -v and above
	if c.v() < signal.V {
		return
	}

	st := c.actionState(name)

	st.mu.Lock()
	defer st.mu.Unlock()

	if st.headerPrinted || st.finished {
		return
	}

	c.outln(
		ansi.Blue.Reg,
		`[action] %s`,
		name,
	)
	st.headerPrinted = true
}

func (c *cli) outln(color ansi.Code, format string, args ...any) {
	c.println(c.out, color, format, args...)
}

func (c *cli) errln(color ansi.Code, format string, args ...any) {
	c.println(c.err, color, format, args...)
}

func (c *cli) println(w io.Writer, color ansi.Code, format string, args ...any) {
	msg := c.paint(color, format, args...)

	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = fmt.Fprintln(w, msg)
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
		return c.isTTY
	default:
		return false
	}
}
