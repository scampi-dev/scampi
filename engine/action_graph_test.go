package engine

import (
	"testing"

	"godoit.dev/doit/spec"
)

// mockAction implements spec.Action for testing
type mockAction struct {
	desc string
	kind string
}

func (m *mockAction) Desc() string   { return m.desc }
func (m *mockAction) Kind() string   { return m.kind }
func (m *mockAction) Ops() []spec.Op { return nil }

// mockPatherAction implements spec.Action and spec.Pather for testing
type mockPatherAction struct {
	desc    string
	kind    string
	inputs  []string
	outputs []string
}

func (m *mockPatherAction) Desc() string          { return m.desc }
func (m *mockPatherAction) Kind() string          { return m.kind }
func (m *mockPatherAction) Ops() []spec.Op        { return nil }
func (m *mockPatherAction) InputPaths() []string  { return m.inputs }
func (m *mockPatherAction) OutputPaths() []string { return m.outputs }

func TestBuildActionGraph_NoDependencies(t *testing.T) {
	// Two Pather actions with no path overlap -> no dependencies
	actions := []spec.Action{
		&mockPatherAction{desc: "A", outputs: []string{"/a"}},
		&mockPatherAction{desc: "B", outputs: []string{"/b"}},
	}

	nodes := buildActionGraph(actions)

	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// Neither should have dependencies
	for _, n := range nodes {
		if len(n.requires) != 0 {
			t.Errorf("action %s should have no dependencies, got %d", n.action.Desc(), len(n.requires))
		}
	}
}

func TestBuildActionGraph_PathDependency(t *testing.T) {
	// A writes /foo, B reads /foo -> B depends on A
	actions := []spec.Action{
		&mockPatherAction{desc: "A", outputs: []string{"/foo"}},
		&mockPatherAction{desc: "B", inputs: []string{"/foo"}, outputs: []string{"/bar"}},
	}

	nodes := buildActionGraph(actions)

	if len(nodes[1].requires) != 1 {
		t.Fatalf("B should have 1 dependency, got %d", len(nodes[1].requires))
	}

	if nodes[1].requires[0] != nodes[0] {
		t.Error("B should depend on A")
	}

	if len(nodes[0].requiredBy) != 1 || nodes[0].requiredBy[0] != nodes[1] {
		t.Error("A should be required by B")
	}
}

func TestBuildActionGraph_NonPatherSequential(t *testing.T) {
	// Non-Pather actions run sequentially
	actions := []spec.Action{
		&mockAction{desc: "A"},
		&mockAction{desc: "B"},
		&mockAction{desc: "C"},
	}

	nodes := buildActionGraph(actions)

	// A has no deps
	if len(nodes[0].requires) != 0 {
		t.Error("A should have no dependencies")
	}

	// B depends on A
	if len(nodes[1].requires) != 1 || nodes[1].requires[0] != nodes[0] {
		t.Error("B should depend on A")
	}

	// C depends on B
	if len(nodes[2].requires) != 1 || nodes[2].requires[0] != nodes[1] {
		t.Error("C should depend on B")
	}
}

func TestBuildActionGraph_MixedPatherAndNonPather(t *testing.T) {
	// Pather actions are independent, non-Pather are sequential among themselves
	actions := []spec.Action{
		&mockPatherAction{desc: "P1", outputs: []string{"/p1"}},
		&mockAction{desc: "N1"},
		&mockPatherAction{desc: "P2", outputs: []string{"/p2"}},
		&mockAction{desc: "N2"},
	}

	nodes := buildActionGraph(actions)

	// P1: no deps
	if len(nodes[0].requires) != 0 {
		t.Error("P1 should have no dependencies")
	}

	// N1: no deps (first non-Pather)
	if len(nodes[1].requires) != 0 {
		t.Error("N1 should have no dependencies")
	}

	// P2: no deps (independent Pather)
	if len(nodes[2].requires) != 0 {
		t.Error("P2 should have no dependencies")
	}

	// N2: depends on N1 (second non-Pather)
	if len(nodes[3].requires) != 1 || nodes[3].requires[0] != nodes[1] {
		t.Error("N2 should depend on N1")
	}
}

func TestBuildActionGraph_ChainedDependencies(t *testing.T) {
	// A -> B -> C chain via paths
	actions := []spec.Action{
		&mockPatherAction{desc: "A", outputs: []string{"/a"}},
		&mockPatherAction{desc: "B", inputs: []string{"/a"}, outputs: []string{"/b"}},
		&mockPatherAction{desc: "C", inputs: []string{"/b"}, outputs: []string{"/c"}},
	}

	nodes := buildActionGraph(actions)

	// A: no deps
	if len(nodes[0].requires) != 0 {
		t.Error("A should have no dependencies")
	}

	// B: depends on A
	if len(nodes[1].requires) != 1 || nodes[1].requires[0] != nodes[0] {
		t.Error("B should depend on A")
	}

	// C: depends on B
	if len(nodes[2].requires) != 1 || nodes[2].requires[0] != nodes[1] {
		t.Error("C should depend on B")
	}
}

func TestInitActionPending(t *testing.T) {
	actions := []spec.Action{
		&mockPatherAction{desc: "A", outputs: []string{"/foo"}},
		&mockPatherAction{desc: "B", inputs: []string{"/foo"}},
	}

	nodes := buildActionGraph(actions)
	initActionPending(nodes)

	if nodes[0].pending != 0 {
		t.Errorf("A should have pending=0, got %d", nodes[0].pending)
	}

	if nodes[1].pending != 1 {
		t.Errorf("B should have pending=1, got %d", nodes[1].pending)
	}
}
