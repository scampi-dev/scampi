// SPDX-License-Identifier: GPL-3.0-only

package layout

import (
	"strings"
	"testing"
)

func TestElideTail(t *testing.T) {
	cases := []struct {
		in, want string
		w        int
	}{
		{"abcdef", "abcdef", 6}, // fits, untouched
		{"abcdef", "abcd…", 5},  // one over -> elide
		{"abcdefgh", "abc…", 4},
		{"abcdefgh", "a…", 2},
	}
	for _, c := range cases {
		if got := elideTail(c.in, c.w); got != c.want {
			t.Errorf("elideTail(%q,%d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
}

func TestElideMiddle_PreservesTail(t *testing.T) {
	// A path: the distinguishing filename at the end must survive.
	got := elideMiddle("/tmp/scampi-sandbox/index.html", 14)
	if !strings.HasSuffix(got, ".html") {
		t.Errorf("middle elide dropped the tail: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("expected an ellipsis: %q", got)
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
	line, w, minW := Fit(row("│┏━", "[1] copy", "(detail text here)"), 60, 1)
	for _, want := range []string{"│┏━", "[1] copy", "(detail text here)"} {
		if !strings.Contains(line, want) {
			t.Errorf("wide line missing %q: %q", want, line)
		}
	}
	if w != VisibleLen(line) {
		t.Errorf("reported width %d != actual %d", w, VisibleLen(line))
	}
	if minW != VisibleLen("[1] copy") {
		t.Errorf("minW = %d, want %d", minW, VisibleLen("[1] copy"))
	}
}

func TestFit_DetailElidesFirst(t *testing.T) {
	line, w, _ := Fit(row("│┏━", "[1] copy", "(/tmp/scampi-sandbox/index.html)"), 26, 1)
	if w > 26 {
		t.Fatalf("over budget: %q (%d)", line, w)
	}
	for _, want := range []string{"│┏━", "[1] copy"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing protected col %q: %q", want, line)
		}
	}
	if !strings.Contains(line, "…") {
		t.Errorf("expected detail to elide: %q", line)
	}
}

func TestFit_DetailDropsBelowFloor(t *testing.T) {
	// Too tight for a useful detail: it must vanish, not render "a…".
	line, _, _ := Fit(row("│┏━", "[1] copy", "(/tmp/scampi-sandbox/index.html)"), 13, 1)
	if strings.Contains(line, "…") {
		t.Errorf("detail should have dropped, not stubbed: %q", line)
	}
	if !strings.Contains(line, "[1] copy") {
		t.Errorf("label must survive: %q", line)
	}
}

func TestFit_GutterDropsAfterDetail(t *testing.T) {
	// Tighter still: detail is already gone, so the gutter goes before the label.
	line, _, _ := Fit(row("│┏━", "[1] copy", "(detail)"), 8, 1)
	if strings.Contains(line, "│┏━") {
		t.Errorf("gutter should have dropped: %q", line)
	}
	if !strings.Contains(line, "[1] copy") {
		t.Errorf("label must survive: %q", line)
	}
}

func TestFit_FloorReportsMinWidth(t *testing.T) {
	// Below the floor the label can't fit; minW exceeds budget so the caller warns.
	_, _, minW := Fit(row("│┏━", "[1] copy", "(detail)"), 5, 1)
	if minW <= 5 {
		t.Errorf("minW = %d, expected > 5 (floor breached)", minW)
	}
}
