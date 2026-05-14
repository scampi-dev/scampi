// SPDX-License-Identifier: GPL-3.0-only

// Scope: cross-cutting tests for the generic cycle-detector used by
// both the action graph and the op DAG.
// Exercises: cycles.go (detectCycles parameterised on node type). Uses
// string-keyed test graphs as the simplest fixture; the same function
// is invoked with *actionNode and *opNode in production.

package engine

import (
	"fmt"
	"testing"
)

func TestDetectCycles_NoCycle(t *testing.T) {
	// A -> B -> C (linear)
	adj := map[string][]string{
		"A": {"B"},
		"B": {"C"},
		"C": nil,
	}
	cycles := detectCycles([]string{"A"}, func(n string) []string { return adj[n] })
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d", len(cycles))
	}
}

func TestDetectCycles_SimpleCycle(t *testing.T) {
	// A -> B -> A
	adj := map[string][]string{
		"A": {"B"},
		"B": {"A"},
	}
	cycles := detectCycles([]string{"A", "B"}, func(n string) []string { return adj[n] })
	if len(cycles) == 0 {
		t.Fatal("expected at least one cycle")
	}

	// Verify the cycle closes: last element == first element
	c := cycles[0]
	if c[0] != c[len(c)-1] {
		t.Errorf("cycle should close: first=%s last=%s", c[0], c[len(c)-1])
	}
}

func TestDetectCycles_SelfLoop(t *testing.T) {
	adj := map[string][]string{
		"A": {"A"},
	}
	cycles := detectCycles([]string{"A"}, func(n string) []string { return adj[n] })
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(cycles))
	}
	if len(cycles[0]) != 2 { // [A, A]
		t.Errorf("expected cycle length 2, got %d", len(cycles[0]))
	}
}

func TestDetectCycles_MultipleCycles(t *testing.T) {
	// Two independent cycles: A->B->A and C->D->C
	adj := map[string][]string{
		"A": {"B"},
		"B": {"A"},
		"C": {"D"},
		"D": {"C"},
	}
	cycles := detectCycles([]string{"A", "B", "C", "D"}, func(n string) []string { return adj[n] })
	if len(cycles) < 2 {
		t.Errorf("expected at least 2 cycles, got %d", len(cycles))
	}
}

func TestDetectCycles_DiamondNoCycle(t *testing.T) {
	// A -> B, A -> C, B -> D, C -> D (diamond, no cycle)
	adj := map[string][]string{
		"A": {"B", "C"},
		"B": {"D"},
		"C": {"D"},
		"D": nil,
	}
	cycles := detectCycles([]string{"A"}, func(n string) []string { return adj[n] })
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d", len(cycles))
	}
}

func TestDetectCycles_UnreachableNodes(t *testing.T) {
	// Cycle exists but only if we start from the right root
	adj := map[string][]string{
		"A": nil,
		"B": {"C"},
		"C": {"B"},
	}
	// Only root A — cycle B->C->B is unreachable
	cycles := detectCycles([]string{"A"}, func(n string) []string { return adj[n] })
	if len(cycles) != 0 {
		t.Errorf("expected no cycles from root A, got %d", len(cycles))
	}

	// All roots — cycle is found
	cycles = detectCycles([]string{"A", "B", "C"}, func(n string) []string { return adj[n] })
	if len(cycles) == 0 {
		t.Error("expected cycle when B is a root")
	}
}

func TestDedupCycles_RemovesRotations(t *testing.T) {
	// Two representations of the same cycle: [A,B,C,A] and [B,C,A,B]
	cycles := [][]string{
		{"A", "B", "C", "A"},
		{"B", "C", "A", "B"},
	}
	deduped := dedupCycles(cycles, func(n string) string { return n })
	if len(deduped) != 1 {
		t.Errorf("expected 1 unique cycle, got %d", len(deduped))
	}
}

func TestDedupCycles_KeepsDistinct(t *testing.T) {
	// Two genuinely different cycles
	cycles := [][]string{
		{"A", "B", "A"},
		{"C", "D", "C"},
	}
	deduped := dedupCycles(cycles, func(n string) string { return n })
	if len(deduped) != 2 {
		t.Errorf("expected 2 unique cycles, got %d", len(deduped))
	}
}

func TestDedupCycles_Empty(t *testing.T) {
	deduped := dedupCycles[string](nil, func(n string) string { return n })
	if len(deduped) != 0 {
		t.Errorf("expected 0 cycles, got %d", len(deduped))
	}
}

func TestDetectCycles_IntNodes(t *testing.T) {
	// Verify generics work with non-string types
	adj := map[int][]int{
		1: {2},
		2: {3},
		3: {1},
	}
	cycles := detectCycles([]int{1, 2, 3}, func(n int) []int { return adj[n] })
	if len(cycles) == 0 {
		t.Fatal("expected a cycle")
	}
	c := cycles[0]
	if c[0] != c[len(c)-1] {
		t.Errorf("cycle should close: first=%d last=%d", c[0], c[len(c)-1])
	}
}

func TestRotationKey_Deterministic(t *testing.T) {
	id := func(n string) string { return n }

	k1 := rotationKey([]string{"A", "B", "C", "A"}, id)
	k2 := rotationKey([]string{"B", "C", "A", "B"}, id)
	k3 := rotationKey([]string{"C", "A", "B", "C"}, id)

	if k1 != k2 || k2 != k3 {
		t.Errorf("rotation keys should match: %q %q %q", k1, k2, k3)
	}
}

func TestPtrKey(t *testing.T) {
	type node struct{ name string }
	n := &node{name: "test"}
	key := ptrKey(n)
	expected := fmt.Sprintf("%p", n)
	if key != expected {
		t.Errorf("ptrKey = %q, want %q", key, expected)
	}
}
