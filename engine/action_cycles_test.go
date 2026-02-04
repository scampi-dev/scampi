package engine

import (
	"testing"

	"godoit.dev/doit/spec"
)

func TestDetectActionCycles_NoCycle(t *testing.T) {
	// Linear chain: A -> B -> C
	actions := []spec.Action{
		&mockPatherAction{desc: "A", outputs: []string{"/a"}},
		&mockPatherAction{desc: "B", inputs: []string{"/a"}, outputs: []string{"/b"}},
		&mockPatherAction{desc: "C", inputs: []string{"/b"}},
	}

	nodes := buildActionGraph(actions)
	cycles := detectActionCycles(nodes)

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
	cycles := detectActionCycles(nodes)

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
	cycles := detectActionCycles(nodes)

	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d", len(cycles))
	}
}

func TestActionCyclicDependency_Error(t *testing.T) {
	a := &mockPatherAction{desc: "action-A"}
	b := &mockPatherAction{desc: "action-B"}

	err := ActionCyclicDependency{
		Cycle: []spec.Action{a, b, a},
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}

	// Should contain action descriptions
	if !contains(errStr, "action-A") || !contains(errStr, "action-B") {
		t.Errorf("error should contain action descriptions: %s", errStr)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
