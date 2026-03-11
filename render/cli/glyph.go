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

	planStart            string
	planRail             string
	planEnd              string
	actionStart          string
	actionStartNoOp      string
	actionStartCollapsed string
	actionRail           string
	actionIndent         string
	actionKindSep        string
	actionEnd            string
	opBranch             string
	opLast               string

	depsArrow string

	parallelTop   string
	parallelMid   string
	parallelBot   string
	parallelLabel string

	spinnerFrames []string
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

		planStart:            "┌─┬",
		planRail:             "│",
		planEnd:              "└─■",
		actionStart:          "┏━┯",
		actionStartNoOp:      "┏━━",
		actionStartCollapsed: "•",
		actionRail:           "┇",
		actionIndent:         "  ",
		actionKindSep:        "›",
		actionEnd:            "■",
		opBranch:             "├─",
		opLast:               "└─",

		depsArrow: "←",

		parallelTop:   "╮",
		parallelMid:   "│",
		parallelBot:   "╯",
		parallelLabel: "⏸",

		spinnerFrames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
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

		planStart:            "+--",
		planRail:             "|",
		planEnd:              "+-#",
		actionStart:          "+--",
		actionStartNoOp:      "+--",
		actionStartCollapsed: "*",
		actionRail:           "|",
		actionIndent:         "  ",
		actionKindSep:        ">",
		actionEnd:            "#",
		opBranch:             "|-",
		opLast:               "`-",

		depsArrow: "<-",

		parallelTop:   ")",
		parallelMid:   ")",
		parallelBot:   ")",
		parallelLabel: "\"",

		spinnerFrames: []string{"|", "/", "-", "\\"},
	}
)

func glyphR(g string) string { return " " + g }
func glyphL(g string) string { return g + " " }
