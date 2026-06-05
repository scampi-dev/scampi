// SPDX-License-Identifier: GPL-3.0-only

package main

import "os"

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
}

var (
	UnicodeGlyphs = Glyphs{
		Create:  "✚", // heavy greek cross
		Update:  "↻", // clockwise open circle arrow
		Destroy: "⊖", // circled minus
		InSync:  "✓", // check mark
		Halt:    "⊘", // circled division slash
		Adopt:   "⊕", // circled plus
	}
	ASCIIGlyphs = Glyphs{
		Create:  "+",
		Update:  "~",
		Destroy: "-",
		InSync:  "=",
		Halt:    "!",
		Adopt:   "@",
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
