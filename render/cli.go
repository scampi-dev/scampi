package render

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"godoit.dev/doit/signal"
)

type (
	ANSI       string
	CLIOptions struct {
		ColorMode ColorMode
	}
	cli struct {
		opts  CLIOptions
		out   io.Writer
		err   io.Writer
		isTTY bool
	}
)

const (
	Reset ANSI = "\033[0m"

	// styles
	Bold ANSI = "\033[1m"
	Dim  ANSI = "\033[2m"

	// base colors
	Black   ANSI = "\033[30m"
	Red     ANSI = "\033[31m"
	Green   ANSI = "\033[32m"
	Yellow  ANSI = "\033[33m"
	Blue    ANSI = "\033[34m"
	Magenta ANSI = "\033[35m"
	Cyan    ANSI = "\033[36m"
	White   ANSI = "\033[37m"

	BrightBlack   ANSI = "\033[90m"
	BrightRed     ANSI = "\033[91m"
	BrightGreen   ANSI = "\033[92m"
	BrightYellow  ANSI = "\033[93m"
	BrightBlue    ANSI = "\033[94m"
	BrightMagenta ANSI = "\033[95m"
	BrightCyan    ANSI = "\033[96m"
	BrightWhite   ANSI = "\033[97m"
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
// =============================================

func (c *cli) EngineStart(s signal.Severity) {
	col := ColorsForSeverity(s)
	c.outln(col.Normal, "[engine] starting")
}

func (c *cli) EngineFinish(s signal.Severity, nChanged, nUnits int, duration time.Duration) {
	col := ColorsForSeverity(s)
	units := pluralS(nUnits, "unit")
	if nChanged > 0 {
		c.outln(col.Highlight, "[engine] finished (%s, %d %s, %d %s)", duration, nChanged, pluralS(nChanged, "change"), nUnits, units)
	} else {
		c.outln(col.Dimmed, "[engine] finished (%s, no changes, %d %s)", duration, nUnits, units)
	}
}

// Config / planning phase
// =============================================

func (c *cli) PlanStart(s signal.Severity) {
	col := ColorsForSeverity(s)
	c.outln(col.Normal, "[plan] start")
}

func (c *cli) UnitPlanned(s signal.Severity, index int, name, kind string) {
	col := ColorsForSeverity(s)
	c.outln(col.Dimmed, "  [unit] #%d %s (%s)", index, name, kind)
}

func (c *cli) PlanFinish(s signal.Severity, unitCount int, duration time.Duration) {
	col := ColorsForSeverity(s)
	c.outln(col.Dimmed, "[plan] %d %s (%s)", unitCount, pluralS(unitCount, "unit"), duration)
}

// Action lifecycle
// =============================================

func (c *cli) ActionStart(s signal.Severity, name string) {
	col := ColorsForSeverity(s)
	c.outln(col.Normal, "[action] %s", name)
}

func (c *cli) ActionFinish(s signal.Severity, name string, changed bool, duration time.Duration) {
	col := ColorsForSeverity(s)
	if changed {
		c.outln(col.Highlight, "[action] %s changed (%s)", name, duration)
	} else {
		c.outln(col.Dimmed, "[action] %s up-to-date", name)
	}
}

func (c *cli) ActionError(s signal.Severity, name string, err error) {
	col := ColorsForSeverity(s)
	c.errln(col.Highlight, "  [action] %s failed: %v", name, err)
}

// Ops signals
// =============================================

func (c *cli) OpCheckStart(s signal.Severity, action, op string) {
	col := ColorsForSeverity(s)
	c.outln(col.Dimmed, "  [check] %s/%s", action, op)
}

func (c *cli) OpCheckSatisfied(s signal.Severity, action, op string) {
	col := ColorsForSeverity(s)
	c.outln(col.Dimmed, "  [check] %s/%s ✓", action, op)
}

func (c *cli) OpCheckUnsatisfied(s signal.Severity, action, op string) {
	col := ColorsForSeverity(s)
	c.outln(col.Normal, "  [check] %s/%s needs change", action, op)
}

func (c *cli) OpCheckUnknown(s signal.Severity, action, op string, err error) {
	col := ColorsForSeverity(s)
	c.errln(col.Highlight, "  [check] %s/%s unknown: %v", action, op, err)
}

func (c *cli) OpExecuteStart(s signal.Severity, action, op string) {
	col := ColorsForSeverity(s)
	c.outln(col.Dimmed, "  [exec] %s/%s", action, op)
}

func (c *cli) OpExecuteFinish(s signal.Severity, action, op string, changed bool, d time.Duration) {
	col := ColorsForSeverity(s)
	if changed {
		c.outln(col.Normal, "  [exec] %s/%s changed (%s)", action, op, d)
	} else {
		c.outln(col.Dimmed, "  [exec] %s/%s no-op", action, op)
	}
}

func (c *cli) OpExecuteError(s signal.Severity, action, op string, err error) {
	col := ColorsForSeverity(s)
	c.errln(col.Highlight, "  [exec] %s/%s failed: %v", action, op, err)
}

// User-visible errors (expected, actionable)
// =============================================

func (c *cli) UserError(s signal.Severity, message string) {
	col := ColorsForSeverity(s)
	c.errln(col.Normal, "[error] %s", message)
}

// Internal errors (bugs, invariants violated)
// =============================================

func (c *cli) InternalError(s signal.Severity, message string, err error) {
	col := ColorsForSeverity(s)
	if err != nil {
		c.errln(col.Highlight, "[fatal] %s: %v", message, err)
	} else {
		c.errln(col.Highlight, "[fatal] %s", message)
	}
}

// Internal helpers
// =============================================

func (c *cli) outln(color Color, format string, args ...any) {
	c.println(c.out, string(colToANSI(color)), format, args...)
}

func (c *cli) errln(color Color, format string, args ...any) {
	c.println(c.err, string(colToANSI(color)), format, args...)
}

func (c *cli) println(w io.Writer, color string, format string, args ...any) {
	msg := c.paint(color, format, args...)
	_, _ = fmt.Fprintln(w, msg)
}

func (c *cli) paint(color string, format string, args ...any) string {
	if !c.shouldUseColor() {
		return fmt.Sprintf(format, args...)
	}
	return color + fmt.Sprintf(format, args...) + string(Reset)
}

func (c *cli) shouldUseColor() bool {
	switch c.opts.ColorMode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	case ColorAuto:
		return c.isTTY
	default:
		return false
	}
}

func colToANSI(c Color) ANSI {
	switch c {

	case DebugHighlight:
		return BrightBlack + Bold
	case DebugNormal:
		return BrightBlack
	case DebugDimmed:
		return BrightBlack + Dim

	case InfoHighlight:
		return Blue + Bold
	case InfoNormal:
		return Blue
	case InfoDimmed:
		return Blue + Dim

	case NoticeHighlight:
		return Cyan + Bold
	case NoticeNormal:
		return Cyan
	case NoticeDimmed:
		return Cyan + Dim

	case ImportantHighlight:
		return Green + Bold
	case ImportantNormal:
		return Green
	case ImportantDimmed:
		return Green + Dim

	case WarningHighlight:
		return Yellow + Bold
	case WarningNormal:
		return Yellow
	case WarningDimmed:
		return Yellow + Dim

	case ErrorHighlight:
		return Red + Bold
	case ErrorNormal:
		return Red
	case ErrorDimmed:
		return Red + Dim

	case FatalHighlight:
		return BrightRed + Bold
	case FatalNormal:
		return BrightRed
	case FatalDimmed:
		return BrightRed + Dim

	default:
		panic("unhandled Color")
	}
}
