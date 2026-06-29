// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"slices"
	"testing"

	"scampi.dev/scampi/internal/spec"
)

type mockStep struct {
	desc string
	kind string
	ops  []spec.Op
}

func (m *mockStep) Desc() string   { return m.desc }
func (m *mockStep) Kind() string   { return m.kind }
func (m *mockStep) Ops() []spec.Op { return m.ops }

// mockPromiserStep implements spec.Step and spec.Promiser for testing
type mockPromiserStep struct {
	desc     string
	kind     string
	inputs   []spec.Resource
	promises []spec.Resource
}

func (m *mockPromiserStep) Desc() string              { return m.desc }
func (m *mockPromiserStep) Kind() string              { return m.kind }
func (m *mockPromiserStep) Ops() []spec.Op            { return nil }
func (m *mockPromiserStep) Inputs() []spec.Resource   { return m.inputs }
func (m *mockPromiserStep) Promises() []spec.Resource { return m.promises }

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

func labels(s ...string) []spec.Resource {
	r := make([]spec.Resource, len(s))
	for i, l := range s {
		r[i] = spec.LabelResource(l)
	}
	return r
}

func TestBuildStepGraph_NoDependencies(t *testing.T) {
	// Two Promiser steps with no path overlap -> no dependencies
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", promises: paths("/a")},
		&mockPromiserStep{desc: "B", promises: paths("/b")},
	}

	nodes := buildStepGraph(steps)

	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	for _, n := range nodes {
		noDeps(t, n, n.step.Desc())
	}
}

func TestBuildStepGraph_PathDependency(t *testing.T) {
	// A writes /foo, B reads /foo -> B depends on A
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", promises: paths("/foo")},
		&mockPromiserStep{desc: "B", inputs: paths("/foo"), promises: paths("/bar")},
	}

	nodes := buildStepGraph(steps)

	requiresExactly(t, nodes[1], "B", nodes[0])

	// requiredBy is the reverse edge; one direct probe keeps coverage that the
	// graph is bidirectional without growing another helper for a single use.
	if len(nodes[0].requiredBy) != 1 || nodes[0].requiredBy[0] != nodes[1] {
		t.Error("A should be required by B")
	}
}

func TestBuildStepGraph_NonPatherSequential(t *testing.T) {
	// Consecutive barriers chain: A→B→C (transitive ordering, O(n) edges)
	steps := []spec.Step{
		&mockStep{desc: "A"},
		&mockStep{desc: "B"},
		&mockStep{desc: "C"},
	}

	nodes := buildStepGraph(steps)

	noDeps(t, nodes[0], "A")
	requiresExactly(t, nodes[1], "B", nodes[0])
	requiresExactly(t, nodes[2], "C", nodes[1])
}

func TestBuildStepGraph_NonPatherBarrier(t *testing.T) {
	// Fence semantics: barriers chain and fan in/out to neighboring path
	// nodes. P1→N1→P2→N2 with fan-in edges from Pathers between barriers.
	steps := []spec.Step{
		&mockPromiserStep{desc: "P1", promises: paths("/p1")},
		&mockStep{desc: "N1"},
		&mockPromiserStep{desc: "P2", promises: paths("/p2")},
		&mockStep{desc: "N2"},
	}

	nodes := buildStepGraph(steps)

	noDeps(t, nodes[0], "P1")
	// N1: fan-in from Pathers before this barrier
	requiresExactly(t, nodes[1], "N1", nodes[0])
	// P2: fan-out from preceding barrier
	requiresExactly(t, nodes[2], "P2", nodes[1])
	// N2: chain through N1, fan-in via P2; P1 reached transitively
	requiresExactly(t, nodes[3], "N2", nodes[1], nodes[2])
}

// requiresExactly and noDeps are the only places that read stepNode.requires.
// Keep dependency assertions routed through these helpers so any future rename
// of the unexported field touches one spot, not every test.
func requiresExactly(t *testing.T, n *stepNode, name string, expected ...*stepNode) {
	t.Helper()
	if len(n.requires) != len(expected) {
		t.Errorf("%s: expected %d dependencies, got %d", name, len(expected), len(n.requires))
		return
	}
	for _, e := range expected {
		if !slices.Contains(n.requires, e) {
			t.Errorf("%s: missing expected dependency on %s", name, e.step.Desc())
		}
	}
}

func noDeps(t *testing.T, n *stepNode, name string) {
	t.Helper()
	if len(n.requires) != 0 {
		t.Errorf("%s: expected no dependencies, got %d", name, len(n.requires))
	}
}

func TestBuildStepGraph_ChainedDependencies(t *testing.T) {
	// A -> B -> C chain via paths
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", promises: paths("/a")},
		&mockPromiserStep{desc: "B", inputs: paths("/a"), promises: paths("/b")},
		&mockPromiserStep{desc: "C", inputs: paths("/b"), promises: paths("/c")},
	}

	nodes := buildStepGraph(steps)

	noDeps(t, nodes[0], "A")
	requiresExactly(t, nodes[1], "B", nodes[0])
	requiresExactly(t, nodes[2], "C", nodes[1])
}

func TestBuildStepGraph_ParentDirDependency(t *testing.T) {
	// dir creates /home/user/.ssh, copy writes /home/user/.ssh/authorized_keys
	// -> copy should depend on dir (parent directory)
	steps := []spec.Step{
		&mockPromiserStep{desc: "dir", promises: paths("/home/user/.ssh")},
		&mockPromiserStep{
			desc: "copy", inputs: paths("./keys"),
			promises: paths("/home/user/.ssh/authorized_keys"),
		},
	}

	nodes := buildStepGraph(steps)

	requiresExactly(t, nodes[1], "copy", nodes[0])
}

func TestBuildStepGraph_UserDependency(t *testing.T) {
	// user step promises user "app", dir step consumes user "app" -> dependency
	steps := []spec.Step{
		&mockPromiserStep{desc: "user", promises: users("app")},
		&mockPromiserStep{desc: "dir", inputs: users("app"), promises: paths("/opt/app")},
	}

	nodes := buildStepGraph(steps)
	requiresExactly(t, nodes[1], "dir", nodes[0])
}

func TestBuildStepGraph_GroupDependency(t *testing.T) {
	// group step promises group "staff", dir step consumes group "staff" -> dependency
	steps := []spec.Step{
		&mockPromiserStep{desc: "group", promises: groups("staff")},
		&mockPromiserStep{desc: "dir", inputs: groups("staff"), promises: paths("/srv")},
	}

	nodes := buildStepGraph(steps)
	requiresExactly(t, nodes[1], "dir", nodes[0])
}

func TestBuildStepGraph_CrossKindIndependent(t *testing.T) {
	// A promises path "/foo", B consumes user "foo" -> no dependency (different kinds)
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", promises: paths("/foo")},
		&mockPromiserStep{desc: "B", inputs: users("foo"), promises: paths("/bar")},
	}

	nodes := buildStepGraph(steps)
	noDeps(t, nodes[1], "B")
}

func TestBuildStepGraph_UserPromiserNotBarrier(t *testing.T) {
	// A user step with resources is NOT a barrier — parallel path steps
	// should not be serialized through it.
	// P1, user, P2 with no resource overlap: P1 and P2 run in parallel,
	// user is not a barrier because it has resources (user promise).
	steps := []spec.Step{
		&mockPromiserStep{desc: "P1", promises: paths("/a")},
		&mockPromiserStep{desc: "user", promises: users("app")},
		&mockPromiserStep{desc: "P2", promises: paths("/b")},
	}

	nodes := buildStepGraph(steps)

	noDeps(t, nodes[0], "P1")
	noDeps(t, nodes[1], "user")
	noDeps(t, nodes[2], "P2")
}

func TestBuildStepGraph_MixedResourceChain(t *testing.T) {
	// group → user (consumes group) → dir (consumes user and path)
	steps := []spec.Step{
		&mockPromiserStep{desc: "group", promises: groups("staff")},
		&mockPromiserStep{desc: "user", inputs: groups("staff"), promises: users("app")},
		&mockPromiserStep{desc: "dir", inputs: users("app"), promises: paths("/opt/app")},
	}

	nodes := buildStepGraph(steps)

	requiresExactly(t, nodes[1], "user", nodes[0])
	requiresExactly(t, nodes[2], "dir", nodes[1])
}

func TestBuildStepGraph_LabelResource_DistinctIDsParallel(t *testing.T) {
	// Three steps with distinct label slots — no resource overlap
	// and not barriers, so they run in parallel.
	steps := []spec.Step{
		&mockPromiserStep{desc: "node100", promises: labels("node:100")},
		&mockPromiserStep{desc: "node101", promises: labels("node:101")},
		&mockPromiserStep{desc: "node102", promises: labels("node:102")},
	}

	nodes := buildStepGraph(steps)

	for _, n := range nodes {
		noDeps(t, n, n.step.Desc())
	}
}

func TestBuildStepGraph_LabelResource_NotABarrier(t *testing.T) {
	// A label-resource step between two path steps must not act
	// as a barrier (regression test for #235).
	steps := []spec.Step{
		&mockPromiserStep{desc: "P1", promises: paths("/a")},
		&mockPromiserStep{desc: "node", promises: labels("node:100")},
		&mockPromiserStep{desc: "P2", promises: paths("/b")},
	}

	nodes := buildStepGraph(steps)

	for _, n := range nodes {
		noDeps(t, n, n.step.Desc())
	}
}

func TestBuildStepGraph_ParentDirDoesNotApplyToNonPaths(t *testing.T) {
	// Parent-directory prefix matching only applies to path resources.
	// user "app" should NOT create a dependency on user "app/sub".
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", promises: users("app")},
		&mockPromiserStep{desc: "B", promises: users("app/sub")},
	}

	nodes := buildStepGraph(steps)
	noDeps(t, nodes[1], "B")
}

func TestInitStepPending(t *testing.T) {
	steps := []spec.Step{
		&mockPromiserStep{desc: "A", promises: paths("/foo")},
		&mockPromiserStep{desc: "B", inputs: paths("/foo")},
	}

	nodes := buildStepGraph(steps)
	initStepPending(nodes)

	if nodes[0].pending != 0 {
		t.Errorf("A should have pending=0, got %d", nodes[0].pending)
	}

	if nodes[1].pending != 1 {
		t.Errorf("B should have pending=1, got %d", nodes[1].pending)
	}
}
