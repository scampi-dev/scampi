// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"os"
	"strings"
)

type Glyphs struct {
	Create  string
	Update  string
	Destroy string
	InSync  string
	Halt    string
	Adopt   string
	Failed  string
	Rename  string
}

var (
	UnicodeGlyphs = Glyphs{
		Create:  "✚", // heavy greek cross
		Update:  "↻", // clockwise open circle arrow
		Destroy: "󰅖", // nerd-font close (v1's err glyph)
		InSync:  "✓", // check mark
		Halt:    "⏸", // double vertical bar (pause)
		Adopt:   "⤴", // arrow pointing rightwards then curving upwards
		Failed:  "✗", // ballot x
		Rename:  "⇄", // rightwards arrow over leftwards arrow
	}
	ASCIIGlyphs = Glyphs{
		Create:  "+",
		Update:  "~",
		Destroy: "-",
		InSync:  "=",
		Halt:    "!",
		Adopt:   "@",
		Failed:  "x",
		Rename:  ">",
	}
)

// DecideGlyphs forces ASCII if asciiFlag or SCAMPI_ASCII=1; else
// Unicode. No tty-detect: non-UTF8 environments opt in explicitly.
func DecideGlyphs(asciiFlag bool) Glyphs {
	if asciiFlag || os.Getenv("SCAMPI_ASCII") == "1" {
		return ASCIIGlyphs
	}
	return UnicodeGlyphs
}

// indicatorWidth keeps following columns aligned regardless of the
// indicator (INF, ✚, OK, ...).
const indicatorWidth = 3

func padCol(s string, width int) string {
	n := 0
	for range s {
		n++
	}
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
