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
		opts CLIOptions

		mu  sync.Mutex
		out io.Writer
		err io.Writer

		isTTY   bool
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
	symChange = "󰆓 " // modified
	symNoop   = "󰄫 " // up-to-date
	symOK     = "󰄬 " // satisfied
	symExec   = "󰇚 " // executed
	symWarn   = "󰀪 " // warning / unknown
	symFail   = "󰅚 " // failure
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
}

// Planning lifecycle
// ===============================================

func (c *cli) PlanStart(_ signal.Severity) {
	if c.v() >= signal.VV {
		c.outln(
			ansi.Blue.Reg,
			"[plan] start",
		)
	}
}

func (c *cli) UnitPlanned(_ signal.Severity, index int, name, kind string) {
	if c.v() >= signal.VVV {
		c.outln(
			ansi.BrightBlack.Dim,
			"[plan.unit] #%d %s: %s",
			index, kind, name,
		)
	}
}

func (c *cli) PlanFinish(_ signal.Severity, unitCount int, dur time.Duration) {
	if c.v() >= signal.VV {
		c.outln(
			ansi.Blue.Dim,
			"[plan] done: %d unit%s planned (%s)",
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
			"%s %s %s changed (%s)",
			symChange, st.id, name, dur,
		)
		return
	}

	if c.v() >= signal.V {
		c.outln(
			ansi.Green.Dim,
			"%s %s %s up-to-date",
			symNoop, st.id, name,
		)
	}
}

func (c *cli) ActionError(_ signal.Severity, name string, err error) {
	st := c.ensureAction(name)

	c.errln(
		ansi.Red.Reg,
		"%s %s %s failed: %v",
		symFail, st.id, name, err,
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
		"%s %s needs change: %s",
		symChange, st.id, op,
	)
}

func (c *cli) OpCheckSatisfied(_ signal.Severity, action, op string) {
	if c.v() < signal.VVV {
		return
	}

	st := c.ensureAction(action)
	c.outln(
		ansi.BrightBlack.Dim,
		"%s %s %s",
		symOK, st.id, op,
	)
}

func (c *cli) OpCheckUnknown(_ signal.Severity, action, op string, err error) {
	st := c.ensureAction(action)
	c.errln(
		ansi.Yellow.Reg,
		"%s %s check %s unknown: %v",
		symWarn, st.id, op, err,
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
		"%s %s exec %s changed (%s)",
		symExec, st.id, op, dur,
	)
}

func (c *cli) OpExecuteError(_ signal.Severity, action, op string, err error) {
	st := c.ensureAction(action)
	c.errln(
		ansi.Red.Reg,
		"%s %s exec %s failed: %v",
		symFail, st.id, op, err,
	)
}

// Errors
// ===============================================

func (c *cli) UserError(_ signal.Severity, message string) {
	c.errln(
		ansi.Red.Reg,
		"%s error %s",
		symFail, message,
	)
}

func (c *cli) InternalError(_ signal.Severity, message string, err error) {
	if err != nil {
		c.errln(ansi.BrightRed.Bold, "%s fatal %s: %v", symFail, message, err)
		return
	}
	c.errln(ansi.BrightRed.Bold, "%s fatal %s", symFail, message)
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
