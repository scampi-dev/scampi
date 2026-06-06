// SPDX-License-Identifier: GPL-3.0-only

package render

// Glyphs is scampi's indicator set. All values are pre-aligned to
// the same display width; renderers print them verbatim.
type Glyphs struct {
	// Resource sigils.
	Create  string
	Update  string
	Destroy string
	InSync  string
	Halt    string
	Adopt   string
	Failed  string
	Rename  string

	// Log severity tags.
	Info  string
	Debug string
	Warn  string
	Error string

	// Tick outcome.
	OK string
}

var (
	UnicodeGlyphs = Glyphs{
		Create:  "✚  ", // heavy greek cross
		Update:  "↻  ", // clockwise open circle arrow
		Destroy: "󰅖  ", // nerd-font close (v1's err glyph)
		InSync:  "✓  ", // check mark
		Halt:    "⏸  ", // double vertical bar (pause)
		Adopt:   "⤴  ", // arrow pointing rightwards then curving upwards
		Failed:  "✗  ", // ballot x
		Rename:  "⇄  ", // rightwards arrow over leftwards arrow

		Info:  "INF",
		Debug: "DBG",
		Warn:  "WRN",
		Error: "ERR",

		OK: "OK ",
	}
	ASCIIGlyphs = Glyphs{
		Create:  "+  ",
		Update:  "~  ",
		Destroy: "-  ",
		InSync:  "=  ",
		Halt:    "!  ",
		Adopt:   "@  ",
		Failed:  "x  ",
		Rename:  ">  ",

		Info:  "INF",
		Debug: "DBG",
		Warn:  "WRN",
		Error: "ERR",

		OK: "OK ",
	}
)
