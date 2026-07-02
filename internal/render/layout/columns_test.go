// SPDX-License-Identifier: GPL-3.0-only

package layout

import (
	"strings"
	"testing"
)

// The fancy ellipsis and box-drawing gutters appear as escape sequences:
// source stays ASCII while the tests still exercise the real glyph widths.
const (
	fancyEllipsis = "\u2026"
	fancyGutter   = "\u2502\u250f\u2501"
)

func TestElideTail(t *testing.T) {
	cases := []struct {
		in, want string
		w        int
	}{
		{"abcdef", "abcdef", 6},               // fits, untouched
		{"abcdef", "abcd" + fancyEllipsis, 5}, // one over -> elide
		{"abcdefgh", "abc" + fancyEllipsis, 4},
		{"abcdefgh", "a" + fancyEllipsis, 2},
	}
	for _, c := range cases {
		if got := elideTail(c.in, c.w, fancyEllipsis); got != c.want {
			t.Errorf("elideTail(%q,%d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
}

// The ellipsis is caller-supplied so --ascii output never ships the fancy
// one; a wider marker ("...", width 3) must still respect the width budget.
func TestElideTail_ASCIIEllipsis(t *testing.T) {
	got := elideTail("abcdefgh", 6, "...")
	if got != "abc..." {
		t.Errorf("elideTail with ascii ellipsis = %q, want %q", got, "abc...")
	}
	if VisibleLen(got) > 6 {
		t.Errorf("over width: %q (%d > 6)", got, VisibleLen(got))
	}
}

func TestElideMiddle_PreservesTail(t *testing.T) {
	// A path: the distinguishing filename at the end must survive.
	got := elideMiddle("/tmp/scampi-sandbox/index.html", 14, fancyEllipsis)
	if !strings.HasSuffix(got, ".html") {
		t.Errorf("middle elide dropped the tail: %q", got)
	}
	if !strings.Contains(got, fancyEllipsis) {
		t.Errorf("expected an ellipsis: %q", got)
	}
	if VisibleLen(got) > 14 {
		t.Errorf("over width: %q (%d > 14)", got, VisibleLen(got))
	}
}

func TestElideMiddle_ASCIIEllipsis(t *testing.T) {
	got := elideMiddle("/tmp/scampi-sandbox/index.html", 14, "...")
	if !strings.HasSuffix(got, ".html") {
		t.Errorf("middle elide dropped the tail: %q", got)
	}
	if !strings.Contains(got, "...") {
		t.Errorf("expected an ascii ellipsis: %q", got)
	}
	if strings.Contains(got, fancyEllipsis) {
		t.Errorf("fancy ellipsis leaked into ascii elision: %q", got)
	}
	if VisibleLen(got) > 14 {
		t.Errorf("over width: %q (%d > 14)", got, VisibleLen(got))
	}
}

// row builds the canonical plan content row: a droppable gutter, a fixed label,
// and a middle-eliding detail. (The structure column - deps/brackets - is
// placed by the caller, not the primitive.)
func row(gutter, label, detail string) []Col {
	return []Col{
		{Text: gutter, Elide: Drop, MinW: 0, Order: 1},
		{Text: label, Elide: Fixed},
		{Text: detail, Elide: Middle, Order: 3},
	}
}

func TestFit_WideFitsEverything(t *testing.T) {
	line, w := Fit(row(fancyGutter, "[1] copy", "(detail text here)"), 60, 1, fancyEllipsis)
	for _, want := range []string{fancyGutter, "[1] copy", "(detail text here)"} {
		if !strings.Contains(line, want) {
			t.Errorf("wide line missing %q: %q", want, line)
		}
	}
	if w != VisibleLen(line) {
		t.Errorf("reported width %d != actual %d", w, VisibleLen(line))
	}
}

func TestFit_DetailElidesFirst(t *testing.T) {
	line, w := Fit(row(fancyGutter, "[1] copy", "(/tmp/scampi-sandbox/index.html)"), 26, 1, fancyEllipsis)
	if w > 26 {
		t.Fatalf("over budget: %q (%d)", line, w)
	}
	for _, want := range []string{fancyGutter, "[1] copy"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing protected col %q: %q", want, line)
		}
	}
	if !strings.Contains(line, fancyEllipsis) {
		t.Errorf("expected detail to elide: %q", line)
	}
}

func TestFit_DetailDropsBelowFloor(t *testing.T) {
	// Too tight for a useful detail: it must vanish, not render a stub.
	line, _ := Fit(row(fancyGutter, "[1] copy", "(/tmp/scampi-sandbox/index.html)"), 13, 1, fancyEllipsis)
	if strings.Contains(line, fancyEllipsis) {
		t.Errorf("detail should have dropped, not stubbed: %q", line)
	}
	if !strings.Contains(line, "[1] copy") {
		t.Errorf("label must survive: %q", line)
	}
}

func TestFit_GutterDropsAfterDetail(t *testing.T) {
	// Tighter still: detail is already gone, so the gutter goes before the label.
	line, _ := Fit(row(fancyGutter, "[1] copy", "(detail)"), 8, 1, fancyEllipsis)
	if strings.Contains(line, fancyGutter) {
		t.Errorf("gutter should have dropped: %q", line)
	}
	if !strings.Contains(line, "[1] copy") {
		t.Errorf("label must survive: %q", line)
	}
}

func TestFit_BelowFloorStillRendersFixed(t *testing.T) {
	// Budget below what the Fixed columns need: best-effort overflow, never a
	// mangled label. The caller owns the too-narrow warning.
	line, w := Fit(row(fancyGutter, "[1] copy", "(detail)"), 5, 1, fancyEllipsis)
	if !strings.Contains(line, "[1] copy") {
		t.Errorf("label must survive below floor: %q", line)
	}
	if w <= 5 {
		t.Errorf("width = %d, expected overflow past the 5-col budget", w)
	}
}
