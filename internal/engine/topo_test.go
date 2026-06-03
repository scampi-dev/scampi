// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"slices"
	"strings"
	"testing"
)

// Helpers
// -----------------------------------------------------------------------------

func ref(kind, name string) Ref { return Ref{Kind: kind, Name: name} }

func noDeps(Ref) []Ref { return nil }

// Tests
// -----------------------------------------------------------------------------

func TestTopoSort_Empty(t *testing.T) {
	for _, in := range [][]Ref{nil, {}} {
		out, err := topoSort(in, noDeps)
		if err != nil {
			t.Errorf("input %v: unexpected err %v", in, err)
		}
		if len(out) != 0 {
			t.Errorf("input %v: got %+v, want empty", in, out)
		}
	}
}

func TestTopoSort_Chain(t *testing.T) {
	a, b, c := ref("file", "a"), ref("file", "b"), ref("file", "c")
	// a depends on b; b depends on c. Apply order: c, b, a.
	deps := map[Ref][]Ref{a: {b}, b: {c}}
	got, err := topoSort([]Ref{a, b, c}, func(r Ref) []Ref { return deps[r] })
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []Ref{c, b, a}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestTopoSort_Stability(t *testing.T) {
	// Three independent refs: with no edges, ties must preserve input
	// order (Kahn pulls indeg=0 in iteration order).
	a, b, c := ref("file", "a"), ref("file", "b"), ref("file", "c")
	got, err := topoSort([]Ref{a, b, c}, noDeps)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []Ref{a, b, c}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestTopoSort_DepsOutsideSet(t *testing.T) {
	// `a` declares a dep on `x` which is not part of the input. The
	// edge is dropped silently so `a` ends up indeg=0 and gets emitted.
	a, x := ref("file", "a"), ref("file", "x")
	got, err := topoSort([]Ref{a}, func(r Ref) []Ref {
		if r == a {
			return []Ref{x}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !slices.Equal(got, []Ref{a}) {
		t.Errorf("got %+v, want [file.a]", got)
	}
}

func TestTopoSort_Cycle(t *testing.T) {
	a, b := ref("file", "a"), ref("file", "b")
	deps := map[Ref][]Ref{a: {b}, b: {a}}
	_, err := topoSort([]Ref{a, b}, func(r Ref) []Ref { return deps[r] })
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "cycle") {
		t.Errorf("err = %q, want it to mention 'cycle'", msg)
	}
	for _, want := range []string{"file.a", "file.b"} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, want it to name %s", msg, want)
		}
	}
}
