package render

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"godoit.dev/doit/diagnostic"
)

type (
	ColorMode  uint8
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
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever

	reset = "\033[0m"

	bold = "\033[1m"
	dim  = "\033[2m"

	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	gray   = "\033[90m"
	cyan   = "\033[36m"

	boldRed = "\033[1;31m"
)

func NewCLI(opts CLIOptions) diagnostic.Emitter {
	return &cli{
		opts:  opts,
		out:   os.Stdout,
		err:   os.Stderr,
		isTTY: term.IsTerminal(os.Stdout.Fd()),
	}
}

// Engine lifecycle
// =============================================

func (c *cli) EngineStart() {
	c.outln(blue, "[engine] starting")
}

func (c *cli) EngineFinish(nChanged, nUnits int, duration time.Duration) {
	us := "s"
	if nUnits == 1 {
		us = ""
	}

	if nChanged > 0 {
		cs := "s"
		if nChanged == 1 {
			cs = ""
		}

		c.outln(green, "[engine] finished (%s, %d change%s, %d unit%s)", duration, nChanged, cs, nUnits, us)
	} else {
		c.outln(dim, "[engine] finished (%s, no changes, %d unit%s)", duration, nUnits, us)
	}
}

// Config / planning phase
// =============================================

func (c *cli) PlanStart() {
	c.outln(blue, "[plan] start")
}

func (c *cli) UnitPlanned(index int, name, kind string) {
	c.outln(dim, "  [unit] #%d %s (%s)", index, name, kind)
}

func (c *cli) PlanFinish(unitCount int, duration time.Duration) {
	s := "s"
	if unitCount == 1 {
		s = ""
	}
	c.outln(dim, "[plan] %d unit%s (%s)", unitCount, s, duration)
}

// Action lifecycle
// =============================================

func (c *cli) ActionStart(name string) {
	c.outln(bold, "[action] %s", name)
}

func (c *cli) ActionFinish(name string, changed bool, duration time.Duration) {
	if changed {
		c.outln(green, "[action] %s changed (%s)", name, duration)
	} else {
		c.outln(dim, "[action] %s up-to-date", name)
	}
}

func (c *cli) ActionError(name string, err error) {
	c.errln(red, "  [action] %s failed: %v", name, err)
}

// Ops diagnostics
// =============================================

func (c *cli) OpCheckStart(action, op string) {
	c.outln(dim, "  [check] %s/%s", action, op)
}

func (c *cli) OpCheckSatisfied(action, op string) {
	c.outln(dim, "  [check] %s/%s ✓", action, op)
}

func (c *cli) OpCheckUnsatisfied(action, op string) {
	c.outln(cyan, "  [check] %s/%s needs change", action, op)
}

func (c *cli) OpCheckUnknown(action, op string, err error) {
	c.errln(yellow, "  [check] %s/%s unknown: %v", action, op, err)
}

func (c *cli) OpExecuteStart(action, op string) {
	c.outln(dim, "  [exec] %s/%s", action, op)
}

func (c *cli) OpExecuteFinish(action, op string, changed bool, d time.Duration) {
	if changed {
		c.outln(green, "  [exec] %s/%s changed (%s)", action, op, d)
	} else {
		c.outln(dim, "  [exec] %s/%s no-op", action, op)
	}
}

func (c *cli) OpExecuteError(action, op string, err error) {
	c.errln(red, "  [exec] %s/%s failed: %v", action, op, err)
}

// User-visible errors (expected, actionable)
// =============================================

func (c *cli) UserError(message string) {
	c.errln(red, "[error] %s", message)
}

// Internal errors (bugs, invariants violated)
// =============================================

func (c *cli) InternalError(message string, err error) {
	if err != nil {
		c.errln(boldRed, "[fatal] %s: %v", message, err)
	} else {
		c.errln(boldRed, "[fatal] %s", message)
	}
}

// Internal helpers
// =============================================

func (c *cli) outln(color string, format string, args ...any) {
	c.println(c.out, color, format, args...)
}

func (c *cli) errln(color string, format string, args ...any) {
	c.println(c.err, color, format, args...)
}

func (c *cli) println(w io.Writer, color string, format string, args ...any) {
	msg := c.paint(color, format, args...)
	_, _ = fmt.Fprintln(w, msg)
}

func (c *cli) paint(color string, format string, args ...any) string {
	if !c.shouldUseColor() {
		return fmt.Sprintf(format, args...)
	}
	return color + fmt.Sprintf(format, args...) + reset
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
