// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"testing"

	"scampi.dev/scampi/spec"
)

type mockAction struct {
	desc string
	kind string
	ops  []spec.Op
}

func (m *mockAction) Desc() string   { return m.desc }
func (m *mockAction) Kind() string   { return m.kind }
func (m *mockAction) Ops() []spec.Op { return m.ops }

// mockPromiserAction implements spec.Action and spec.Promiser for testing
type mockPromiserAction struct {
	desc     string
	kind     string
	inputs   []spec.Resource
	promises []spec.Resource
}

func (m *mockPromiserAction) Desc() string              { return m.desc }
func (m *mockPromiserAction) Kind() string              { return m.kind }
func (m *mockPromiserAction) Ops() []spec.Op            { return nil }
func (m *mockPromiserAction) Inputs() []spec.Resource   { return m.inputs }
func (m *mockPromiserAction) Promises() []spec.Resource { return m.promises }

func paths(s ...string) []spec.Resource {
	r := make([]spec.Resource, len(s))
	for i, p := range s {
		r[i] = spec.PathResource(p)
	}
	return r
}

func users(s ...string) []spec.Resource {
	r := make([]spec.Resource, len(s))
	for i, u := range s {
		r[i] = spec.UserResource(u)
	}
	return r
}

func groups(s ...string) []spec.Resource {
	r := make([]spec.Resource, len(s))
	for i, g := range s {
		r[i] = spec.GroupResource(g)
	}
	return r
}

func containers(s ...string) []spec.Resource {
	r := make([]spec.Resource, len(s))
	for i, c := range s {
		r[i] = spec.ContainerResource(c)
	}
	return r
}

func TestBuildActionGraph_NoDependencies(t *testing.T) {
	// Two Promiser actions with no path overlap -> no dependencies
	actions := []spec.Action{
		&mockPromiserAction{desc: "A", promises: paths("/a")},
		&mockPromiserAction{desc: "B", promises: paths("/b")},
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
		&mockPromiserAction{desc: "A", promises: paths("/foo")},
		&mockPromiserAction{desc: "B", inputs: paths("/foo"), promises: paths("/bar")},
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
	// Fence semantics: barriers chain and fan in/out to neighboring path
	// nodes. P1→N1→P2→N2 with fan-in edges from Pathers between barriers.
	actions := []spec.Action{
		&mockPromiserAction{desc: "P1", promises: paths("/p1")},
		&mockAction{desc: "N1"},
		&mockPromiserAction{desc: "P2", promises: paths("/p2")},
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
		&mockPromiserAction{desc: "A", promises: paths("/a")},
		&mockPromiserAction{desc: "B", inputs: paths("/a"), promises: paths("/b")},
		&mockPromiserAction{desc: "C", inputs: paths("/b"), promises: paths("/c")},
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
		&mockPromiserAction{desc: "dir", promises: paths("/home/user/.ssh")},
		&mockPromiserAction{
			desc: "copy", inputs: paths("./keys"),
			promises: paths("/home/user/.ssh/authorized_keys"),
		},
	}

	nodes := buildActionGraph(actions)

	requiresExactly(t, nodes[1], "copy", nodes[0])
}

func TestBuildActionGraph_UserDependency(t *testing.T) {
	// user step promises user "app", dir step consumes user "app" -> dependency
	actions := []spec.Action{
		&mockPromiserAction{desc: "user", promises: users("app")},
		&mockPromiserAction{desc: "dir", inputs: users("app"), promises: paths("/opt/app")},
	}

	nodes := buildActionGraph(actions)
	requiresExactly(t, nodes[1], "dir", nodes[0])
}

func TestBuildActionGraph_GroupDependency(t *testing.T) {
	// group step promises group "staff", dir step consumes group "staff" -> dependency
	actions := []spec.Action{
		&mockPromiserAction{desc: "group", promises: groups("staff")},
		&mockPromiserAction{desc: "dir", inputs: groups("staff"), promises: paths("/srv")},
	}

	nodes := buildActionGraph(actions)
	requiresExactly(t, nodes[1], "dir", nodes[0])
}

func TestBuildActionGraph_CrossKindIndependent(t *testing.T) {
	// A promises path "/foo", B consumes user "foo" -> no dependency (different kinds)
	actions := []spec.Action{
		&mockPromiserAction{desc: "A", promises: paths("/foo")},
		&mockPromiserAction{desc: "B", inputs: users("foo"), promises: paths("/bar")},
	}

	nodes := buildActionGraph(actions)
	if len(nodes[1].requires) != 0 {
		t.Errorf("B should not depend on A (different resource kinds), got %d deps", len(nodes[1].requires))
	}
}

func TestBuildActionGraph_UserPromiserNotBarrier(t *testing.T) {
	// A user step with resources is NOT a barrier — parallel path actions
	// should not be serialized through it.
	// P1, user, P2 with no resource overlap: P1 and P2 run in parallel,
	// user is not a barrier because it has resources (user promise).
	actions := []spec.Action{
		&mockPromiserAction{desc: "P1", promises: paths("/a")},
		&mockPromiserAction{desc: "user", promises: users("app")},
		&mockPromiserAction{desc: "P2", promises: paths("/b")},
	}

	nodes := buildActionGraph(actions)

	// P1: no deps
	if len(nodes[0].requires) != 0 {
		t.Error("P1 should have no dependencies")
	}

	// user: no deps (no resource overlap with P1)
	if len(nodes[1].requires) != 0 {
		t.Error("user should have no dependencies")
	}

	// P2: no deps (no resource overlap with P1 or user)
	if len(nodes[2].requires) != 0 {
		t.Error("P2 should have no dependencies")
	}
}

func TestBuildActionGraph_MixedResourceChain(t *testing.T) {
	// group → user (consumes group) → dir (consumes user and path)
	actions := []spec.Action{
		&mockPromiserAction{desc: "group", promises: groups("staff")},
		&mockPromiserAction{desc: "user", inputs: groups("staff"), promises: users("app")},
		&mockPromiserAction{desc: "dir", inputs: users("app"), promises: paths("/opt/app")},
	}

	nodes := buildActionGraph(actions)

	requiresExactly(t, nodes[1], "user", nodes[0])
	requiresExactly(t, nodes[2], "dir", nodes[1])
}

func TestBuildActionGraph_ContainerResource_DistinctIDsParallel(t *testing.T) {
	// Three pve.lxc-style actions with distinct container slots —
	// no resource overlap and not barriers, so they run in parallel.
	actions := []spec.Action{
		&mockPromiserAction{desc: "lxc100", promises: containers("pve://midgard/100")},
		&mockPromiserAction{desc: "lxc101", promises: containers("pve://midgard/101")},
		&mockPromiserAction{desc: "lxc102", promises: containers("pve://midgard/102")},
	}

	nodes := buildActionGraph(actions)

	for _, n := range nodes {
		if len(n.requires) != 0 {
			t.Errorf("%s: expected no dependencies, got %d", n.action.Desc(), len(n.requires))
		}
	}
}

func TestBuildActionGraph_ContainerResource_NotABarrier(t *testing.T) {
	// A container-resource action between two path actions must not act
	// as a barrier (regression test for #235).
	actions := []spec.Action{
		&mockPromiserAction{desc: "P1", promises: paths("/a")},
		&mockPromiserAction{desc: "lxc", promises: containers("pve://midgard/100")},
		&mockPromiserAction{desc: "P2", promises: paths("/b")},
	}

	nodes := buildActionGraph(actions)

	for _, n := range nodes {
		if len(n.requires) != 0 {
			t.Errorf("%s: expected no dependencies, got %d", n.action.Desc(), len(n.requires))
		}
	}
}

func TestBuildActionGraph_ParentDirDoesNotApplyToNonPaths(t *testing.T) {
	// Parent-directory prefix matching only applies to path resources.
	// user "app" should NOT create a dependency on user "app/sub".
	actions := []spec.Action{
		&mockPromiserAction{desc: "A", promises: users("app")},
		&mockPromiserAction{desc: "B", promises: users("app/sub")},
	}

	nodes := buildActionGraph(actions)
	if len(nodes[1].requires) != 0 {
		t.Errorf("B should not depend on A (prefix matching is path-only), got %d deps", len(nodes[1].requires))
	}
}

func TestInitActionPending(t *testing.T) {
	actions := []spec.Action{
		&mockPromiserAction{desc: "A", promises: paths("/foo")},
		&mockPromiserAction{desc: "B", inputs: paths("/foo")},
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
