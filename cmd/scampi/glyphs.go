// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"os"
	"strings"
)

// indicatorWidth is the visible-column width every renderer reserves
// for the leading status indicator (INF, WRN, OK, or an action
// glyph). Holding to a single width keeps every following column
// aligned no matter which indicator the line carries.
const indicatorWidth = 3

// padCol pads s with trailing spaces so its visible column width
// reaches width. Counts runes; assumes single-column runes (our
// glyph set + ASCII tags).
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

// Glyphs are the per-action sigils renderers prefix to per-resource
// lines. Two flavors ship: Unicode (default) and ASCII (fallback for
// limited terminals, recorders, or operator preference).
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

// decideGlyphs resolves the glyph set:
//
//  1. --ascii or SCAMPI_ASCII=1 forces ASCII.
//  2. Otherwise Unicode.
//
// Unlike color, there's no tty-detect: Unicode is well-supported in
// terminals and pipes alike. Operators with non-UTF8 environments
// opt in to ASCII explicitly.
func decideGlyphs(asciiFlag bool) Glyphs {
	if asciiFlag || os.Getenv("SCAMPI_ASCII") == "1" {
		return ASCIIGlyphs
	}
	return UnicodeGlyphs
}
