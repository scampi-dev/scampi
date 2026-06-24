// SPDX-License-Identifier: GPL-3.0-only

package layout

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// MinElidedCols is the floor for an elidable column. If eliding a column would
// leave fewer than this many visible characters (the ellipsis aside), the
// column is dropped entirely rather than rendered as a useless stub like "a…".
const MinElidedCols = 6

// Elide is how a column yields width when a row does not fit the terminal.
type Elide uint8

const (
	// Fixed columns never shrink - the protected payload (labels, structure).
	Fixed Elide = iota
	// Tail keeps the head and cuts the end: "abcdef…".
	Tail
	// Middle keeps both ends, eliding the centre: "abc…xyz". For paths, where
	// the distinguishing tail must survive.
	Middle
	// Drop collapses to MinW filler (a minimal indent), then vanishes entirely.
	// For decorative gutters.
	Drop
)

// Col is one column of a row. When Style is nil, Text is taken as-is (it may
// already carry ANSI color); when Style is non-nil, Text is plain and Style is
// applied to the final, possibly-elided text. The eliding modes (Tail, Middle)
// require plain Text (a Style); pre-colored cols may only be Fixed or Drop.
// Under width pressure columns shrink in descending Order (highest first);
// Fixed columns are never touched.
type Col struct {
	Text  string
	Style func(string) string // nil = Text used as-is (may be pre-colored)
	Elide Elide
	MinW  int // floor visible width when shrinking (Drop: residual indent)
	Order int // shrink order under pressure; higher shrinks first
}

// Fit composes cols into one styled line of at most budget visible columns,
// with sep spaces between adjacent columns, eliding non-Fixed columns (highest
// Order first) as needed. It returns the line, its visible width, and the
// minimum width the Fixed columns need. When budget < min the line is still
// returned best-effort (Fixed columns intact, overflowing) so the caller can
// react - widen the budget, or warn.
func Fit(cols []Col, budget, sep int) (line string, width, minWidth int) {
	minWidth = blockWidth(fixedOnly(cols), sep)

	cols = shrinkToFit(clone(cols), budget, sep)
	line, width = joinCols(cols, sep)
	return line, width, minWidth
}

func clone(cols []Col) []Col {
	out := make([]Col, len(cols))
	copy(out, cols)
	return out
}

func fixedOnly(cols []Col) []Col {
	var out []Col
	for _, c := range cols {
		if c.Elide == Fixed {
			out = append(out, c)
		}
	}
	return out
}

// blockWidth is the visible width of a column block joined by sep.
func blockWidth(cols []Col, sep int) int {
	w := 0
	for _, c := range cols {
		cw := colWidth(c)
		if cw == 0 {
			continue
		}
		if w > 0 {
			w += sep
		}
		w += cw
	}
	return w
}

func colWidth(c Col) int { return VisibleLen(c.Text) }

// shrinkToFit elides/drops the non-Fixed columns, highest Order first, until the
// block fits budget or nothing more can yield.
func shrinkToFit(cols []Col, budget, sep int) []Col {
	if blockWidth(cols, sep) <= budget {
		return cols
	}

	for _, idx := range shrinkOrder(cols) {
		if blockWidth(cols, sep) <= budget {
			break
		}
		over := blockWidth(cols, sep) - budget
		cols[idx] = shrinkCol(cols[idx], colWidth(cols[idx])-over)
	}
	return cols
}

// shrinkOrder returns indices of shrinkable columns, highest Order first.
func shrinkOrder(cols []Col) []int {
	var idx []int
	for i, c := range cols {
		if c.Elide != Fixed {
			idx = append(idx, i)
		}
	}
	// insertion sort by Order desc - column counts are tiny.
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && cols[idx[j]].Order > cols[idx[j-1]].Order; j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	return idx
}

// shrinkCol reduces a column toward targetW, eliding per its mode. A column that
// cannot stay >= MinElidedCols (or its MinW for Drop) is emptied.
func shrinkCol(c Col, targetW int) Col {
	if targetW < 0 {
		targetW = 0
	}
	switch c.Elide {
	case Drop:
		if targetW < c.MinW {
			targetW = 0
		}
		if targetW <= 0 {
			c.Text = ""
		} else {
			c.Text = strings.Repeat(" ", min(targetW, runewidth.StringWidth(c.Text)))
		}
	case Tail:
		if targetW < MinElidedCols {
			c.Text = ""
		} else {
			c.Text = elideTail(c.Text, targetW)
		}
	case Middle:
		if targetW < MinElidedCols {
			c.Text = ""
		} else {
			c.Text = elideMiddle(c.Text, targetW)
		}
	}
	return c
}

// joinCols styles and joins non-empty columns with sep spaces, returning the
// rendered string and its visible width.
func joinCols(cols []Col, sep int) (string, int) {
	var b strings.Builder
	w := 0
	for _, c := range cols {
		if c.Text == "" {
			continue
		}
		if w > 0 {
			b.WriteString(strings.Repeat(" ", sep))
			w += sep
		}
		w += VisibleLen(c.Text)
		if c.Style != nil {
			b.WriteString(c.Style(c.Text))
		} else {
			b.WriteString(c.Text)
		}
	}
	return b.String(), w
}

// elideTail keeps the head, cutting the end with a trailing ellipsis.
func elideTail(s string, maxW int) string {
	if runewidth.StringWidth(s) <= maxW {
		return s
	}
	return takeWidth(s, maxW-1) + "…"
}

// elideMiddle keeps both ends, eliding the centre: head…tail. Bias the surviving
// width toward the tail so distinguishing suffixes (filenames) outlive prefixes.
func elideMiddle(s string, maxW int) string {
	if runewidth.StringWidth(s) <= maxW {
		return s
	}
	budget := maxW - 1 // the ellipsis
	head := budget / 2
	tail := budget - head
	return takeWidth(s, head) + "…" + takeWidthEnd(s, tail)
}

// takeWidth returns the longest prefix of s with visible width <= w.
func takeWidth(s string, w int) string {
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if used+rw > w {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String()
}

// takeWidthEnd returns the longest suffix of s with visible width <= w.
func takeWidthEnd(s string, w int) string {
	rs := []rune(s)
	used := 0
	i := len(rs)
	for i > 0 {
		rw := runewidth.RuneWidth(rs[i-1])
		if used+rw > w {
			break
		}
		used += rw
		i--
	}
	return string(rs[i:])
}
