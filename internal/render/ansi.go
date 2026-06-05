// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"os"

	"github.com/mattn/go-isatty"
)

// Basic 8/16 only so terminal themes survive.
const (
	ansiDim    = "\x1b[2m"
	ansiUndim  = "\x1b[22m"
	ansiBold   = "\x1b[1m"
	ansiDark   = "\x1b[90m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiReset  = "\x1b[39m"
)

// DecideColor priority: always (flag or SCAMPI_COLOR=always) > NO_COLOR > never > tty-detect.
func DecideColor(mode string, w *os.File) bool {
	env := os.Getenv("SCAMPI_COLOR")
	if mode == "always" || env == "always" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if mode == "never" || env == "never" {
		return false
	}
	return isatty.IsTerminal(w.Fd())
}

// Re-exports for the cobra help template in cmd/scampi.
const (
	AnsiBlue   = ansiBlue
	AnsiCyan   = ansiCyan
	AnsiGreen  = ansiGreen
	AnsiRed    = ansiRed
	AnsiYellow = ansiYellow
	AnsiReset  = ansiReset
)
