// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"testing"

	"scampi.dev/scampi/spec"
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
	// Consecutive barriers chain: A→B→C (transitive ordering, O(n) edges)
	actions := []spec.Action{
		&mockAction{desc: "A"},
		&mockAction{desc: "B"},
		&mockAction{desc: "C"},
	}

	nodes := buildActionGraph(actions)

	// A: no deps (first action)
	if len(nodes[0].requires) != 0 {
		t.Error("A should have no dependencies")
	}

	// B: depends on A
	requiresExactly(t, nodes[1], "B", nodes[0])

	// C: depends on B (transitively on A via B→A chain)
	requiresExactly(t, nodes[2], "C", nodes[1])
}

func TestBuildActionGraph_NonPatherBarrier(t *testing.T) {
	// Fence semantics: barriers chain and fan in/out to neighboring Pather
	// nodes. P1→N1→P2→N2 with fan-in edges from Pathers between barriers.
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

	// N1: depends on P1 (fan-in from Pathers before this barrier)
	requiresExactly(t, nodes[1], "N1", nodes[0])

	// P2: depends on N1 (fan-out from preceding barrier)
	requiresExactly(t, nodes[2], "P2", nodes[1])

	// N2: depends on N1 (chain) and P2 (fan-in); P1 reached transitively via N1
	requiresExactly(t, nodes[3], "N2", nodes[1], nodes[2])
}

func requiresExactly(t *testing.T, n *actionNode, name string, expected ...*actionNode) {
	t.Helper()
	if len(n.requires) != len(expected) {
		t.Errorf("%s: expected %d dependencies, got %d", name, len(expected), len(n.requires))
		return
	}
	for _, e := range expected {
		found := false
		for _, r := range n.requires {
			if r == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: missing expected dependency on %s", name, e.action.Desc())
		}
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

func TestBuildActionGraph_ParentDirDependency(t *testing.T) {
	// dir creates /home/user/.ssh, copy writes /home/user/.ssh/authorized_keys
	// -> copy should depend on dir (parent directory)
	actions := []spec.Action{
		&mockPatherAction{desc: "dir", outputs: []string{"/home/user/.ssh"}},
		&mockPatherAction{
			desc: "copy", inputs: []string{"./keys"},
			outputs: []string{"/home/user/.ssh/authorized_keys"},
		},
	}

	nodes := buildActionGraph(actions)

	requiresExactly(t, nodes[1], "copy", nodes[0])
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
