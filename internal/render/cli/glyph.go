// SPDX-License-Identifier: GPL-3.0-only

package cli

type glyphSet struct {
	change string
	ok     string
	exec   string
	warn   string
	err    string
	fatal  string
	hint   string
	help   string
	bullet string
	arrow  string

	planStart          string
	planRail           string
	planEnd            string
	stepStart          string
	stepStartNoOp      string
	stepStartCollapsed string
	stepRail           string
	stepIndent         string
	stepKindSep        string
	stepEnd            string
	opBranch           string
	opLast             string

	emDash    string
	ellipsis  string
	separator string

	depsArrow string

	parallelTop   string
	parallelMid   string
	parallelBot   string
	parallelLabel string
}

var (
	fancyGlyphs = glyphSet{
		change: "󰏫",
		ok:     "󰄬",
		exec:   "󰐊",
		warn:   "󰀦",
		err:    "󰅖",
		fatal:  "󰚌",
		hint:   "󰌵",
		help:   "󰋖",
		bullet: "•",
		arrow:  "→",

		planStart:          "┌─┬",
		planRail:           "│",
		planEnd:            "└─■",
		stepStart:          "┏━┯",
		stepStartNoOp:      "┏━━",
		stepStartCollapsed: "•",
		stepRail:           "┇",
		stepIndent:         "  ",
		stepKindSep:        "›",
		stepEnd:            "■",
		opBranch:           "├─",
		opLast:             "└─",

		emDash:    "—",
		ellipsis:  "…",
		separator: "···",

		depsArrow: "←",

		parallelTop:   "╮",
		parallelMid:   "│",
		parallelBot:   "╯",
		parallelLabel: "⏸",
	}

	asciiGlyphs = glyphSet{
		change: "~",
		ok:     "+",
		exec:   ">",
		warn:   "!",
		err:    "x",
		fatal:  "X",
		hint:   "?",
		help:   "i",
		bullet: "*",
		arrow:  "->",

		planStart:          "+--",
		planRail:           "|",
		planEnd:            "+-#",
		stepStart:          "+--",
		stepStartNoOp:      "+--",
		stepStartCollapsed: "*",
		stepRail:           "|",
		stepIndent:         "  ",
		stepKindSep:        ">",
		stepEnd:            "#",
		opBranch:           "|-",
		opLast:             "`-",

		emDash:    "--",
		ellipsis:  "...",
		separator: "...",

		depsArrow: "<-",

		parallelTop:   ")",
		parallelMid:   ")",
		parallelBot:   ")",
		parallelLabel: "\"",
	}
)

func glyphR(g string) string { return " " + g }
func glyphL(g string) string { return g + " " }
