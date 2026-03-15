// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"strings"
	"testing"

	"scampi.dev/scampi/spec"
)

func detectActionCyclesForTest(nodes []*actionNode) [][]spec.Action {
	rawCycles := dedupCycles(
		detectCycles(nodes, func(n *actionNode) []*actionNode { return n.requires }),
		ptrKey[*actionNode],
	)

	var cycles [][]spec.Action
	for _, raw := range rawCycles {
		cycle := make([]spec.Action, len(raw))
		for i, n := range raw {
			cycle[i] = n.action
		}
		cycles = append(cycles, cycle)
	}
	return cycles
}

func TestDetectActionCycles_NoCycle(t *testing.T) {
	// Linear chain: A -> B -> C
	actions := []spec.Action{
		&mockPatherAction{desc: "A", outputs: []string{"/a"}},
		&mockPatherAction{desc: "B", inputs: []string{"/a"}, outputs: []string{"/b"}},
		&mockPatherAction{desc: "C", inputs: []string{"/b"}},
	}

	nodes := buildActionGraph(actions)
	cycles := detectActionCyclesForTest(nodes)

	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d", len(cycles))
	}
}

func TestDetectActionCycles_SimpleCycle(t *testing.T) {
	// A writes /a, reads /b
	// B writes /b, reads /a
	// -> cycle: A -> B -> A
	actions := []spec.Action{
		&mockPatherAction{desc: "A", inputs: []string{"/b"}, outputs: []string{"/a"}},
		&mockPatherAction{desc: "B", inputs: []string{"/a"}, outputs: []string{"/b"}},
	}

	nodes := buildActionGraph(actions)
	cycles := detectActionCyclesForTest(nodes)

	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(cycles))
	}

	if len(cycles[0]) != 3 { // A -> B -> A (3 elements, last repeats first)
		t.Errorf("expected cycle of length 3, got %d", len(cycles[0]))
	}
}

func TestDetectActionCycles_IndependentActions(t *testing.T) {
	// No path overlap -> no dependencies -> no cycles
	actions := []spec.Action{
		&mockPatherAction{desc: "A", outputs: []string{"/a"}},
		&mockPatherAction{desc: "B", outputs: []string{"/b"}},
		&mockPatherAction{desc: "C", outputs: []string{"/c"}},
	}

	nodes := buildActionGraph(actions)
	cycles := detectActionCyclesForTest(nodes)

	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d", len(cycles))
	}
}

func TestActionCyclicDependency_Error(t *testing.T) {
	a := &mockPatherAction{desc: "action-A"}
	b := &mockPatherAction{desc: "action-B"}

	err := ActionCyclicDependencyError{
		Cycle: []spec.Action{a, b, a},
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}

	// Should contain action descriptions
	if !strings.Contains(errStr, "action-A") || !strings.Contains(errStr, "action-B") {
		t.Errorf("error should contain action descriptions: %s", errStr)
	}
}
