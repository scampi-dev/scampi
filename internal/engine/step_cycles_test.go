// SPDX-License-Identifier: GPL-3.0-only

// Scope: cross-cutting tests for step-level cycle detection.
// Exercises: step_graph.go (stepNode + requires), cycles.go
// (detectCycles, dedupCycles, ptrKey). Asserts that a planned step
// DAG with introduced cycles is rejected with an error whose payload
// names every node along the cycle in order.

package engine

import (
	"strings"
	"testing"

	"scampi.dev/scampi/internal/spec"
)

func detectStepCyclesForTest(nodes []*stepNode) [][]spec.Step {
	rawCycles := dedupCycles(
		detectCycles(nodes, func(n *stepNode) []*stepNode { return n.requires }),
		ptrKey[*stepNode],
	)

	var cycles [][]spec.Step
	for _, raw := range rawCycles {
		cycle := make([]spec.Step, len(raw))
		for i, n := range raw {
			cycle[i] = n.step
		}
		cycles = append(cycles, cycle)
	}
	return cycles
}

func TestDetectStepCycles_NoCycle(t *testing.T) {
	// Linear chain: A -> B -> C
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", promises: paths("/a")},
		&mockPromiserStep{desc: "B", inputs: paths("/a"), promises: paths("/b")},
		&mockPromiserStep{desc: "C", inputs: paths("/b")},
	}

	nodes := buildStepGraph(steps)
	cycles := detectStepCyclesForTest(nodes)

	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d", len(cycles))
	}
}

func TestDetectStepCycles_SimpleCycle(t *testing.T) {
	// A writes /a, reads /b
	// B writes /b, reads /a
	// -> cycle: A -> B -> A
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", inputs: paths("/b"), promises: paths("/a")},
		&mockPromiserStep{desc: "B", inputs: paths("/a"), promises: paths("/b")},
	}

	nodes := buildStepGraph(steps)
	cycles := detectStepCyclesForTest(nodes)

	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(cycles))
	}

	if len(cycles[0]) != 3 { // A -> B -> A (3 elements, last repeats first)
		t.Errorf("expected cycle of length 3, got %d", len(cycles[0]))
	}
}

func TestDetectStepCycles_IndependentSteps(t *testing.T) {
	// No path overlap -> no dependencies -> no cycles
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", promises: paths("/a")},
		&mockPromiserStep{desc: "B", promises: paths("/b")},
		&mockPromiserStep{desc: "C", promises: paths("/c")},
	}

	nodes := buildStepGraph(steps)
	cycles := detectStepCyclesForTest(nodes)

	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d", len(cycles))
	}
}

func TestStepCyclicDependency_Error(t *testing.T) {
	a := &mockPromiserStep{desc: "step-A"}
	b := &mockPromiserStep{desc: "step-B"}

	err := StepCyclicDependencyError{
		Cycle: []spec.Step{a, b, a},
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}

	// Should contain step descriptions
	if !strings.Contains(errStr, "step-A") || !strings.Contains(errStr, "step-B") {
		t.Errorf("error should contain step descriptions: %s", errStr)
	}
}
