// SPDX-License-Identifier: GPL-3.0-only

package render

// Basic 8/16 only so terminal themes survive.
const (
	AnsiDim    = "\x1b[2m"
	AnsiUndim  = "\x1b[22m"
	AnsiBold   = "\x1b[1m"
	AnsiDark   = "\x1b[90m"
	AnsiGreen  = "\x1b[32m"
	AnsiYellow = "\x1b[33m"
	AnsiRed    = "\x1b[31m"
	AnsiBlue   = "\x1b[34m"
	AnsiCyan   = "\x1b[36m"
	AnsiReset  = "\x1b[39m"
)
