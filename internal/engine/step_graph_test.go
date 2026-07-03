// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"slices"
	"testing"

	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/copy"
	"scampi.dev/scampi/internal/step/pkg"
	"scampi.dev/scampi/internal/step/run"
	"scampi.dev/scampi/internal/step/service"
)

type mockStep struct {
	desc string
	kind string
	ops  []spec.Op
}

func (m *mockStep) Desc() string   { return m.desc }
func (m *mockStep) Kind() string   { return m.kind }
func (m *mockStep) Ops() []spec.Op { return m.ops }

// mockResourceStep implements spec.Step and spec.Provider for testing
type mockResourceStep struct {
	desc     string
	kind     string
	requires []spec.Resource
	provides []spec.Resource
}

func (m *mockResourceStep) Desc() string              { return m.desc }
func (m *mockResourceStep) Kind() string              { return m.kind }
func (m *mockResourceStep) Ops() []spec.Op            { return nil }
func (m *mockResourceStep) Requires() []spec.Resource { return m.requires }
func (m *mockResourceStep) Provides() []spec.Resource { return m.provides }

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

func Test_BuildStepGraph_KeepsDisjointPathsIndependent(t *testing.T) {
	// Two provider steps with no path overlap -> no dependencies
	steps := []spec.Step{
		&mockResourceStep{desc: "A", provides: paths("/a")},
		&mockResourceStep{desc: "B", provides: paths("/b")},
	}

	nodes := buildStepGraph(steps)

	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	for _, n := range nodes {
		noDeps(t, n, n.step.Desc())
	}
}

func Test_BuildStepGraph_OrdersPathReaderAfterWriter(t *testing.T) {
	// A writes /foo, B reads /foo -> B depends on A
	steps := []spec.Step{
		&mockResourceStep{desc: "A", provides: paths("/foo")},
		&mockResourceStep{desc: "B", requires: paths("/foo"), provides: paths("/bar")},
	}

	nodes := buildStepGraph(steps)

	requiresExactly(t, nodes[1], "B", nodes[0])

	// requiredBy is the reverse edge; one direct probe keeps coverage that the
	// graph is bidirectional without growing another helper for a single use.
	if len(nodes[0].requiredBy) != 1 || nodes[0].requiredBy[0] != nodes[1] {
		t.Error("A should be required by B")
	}
}

func Test_BuildStepGraph_ChainsBarriersSequentially(t *testing.T) {
	// Consecutive barriers chain: A->B->C (transitive ordering, O(n) edges)
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

func Test_BuildStepGraph_TreatsNonPathersAsFences(t *testing.T) {
	// Fence semantics: barriers chain and fan in/out to neighboring path
	// nodes. P1->N1->P2->N2 with fan-in edges from Pathers between barriers.
	steps := []spec.Step{
		&mockResourceStep{desc: "P1", provides: paths("/p1")},
		&mockStep{desc: "N1"},
		&mockResourceStep{desc: "P2", provides: paths("/p2")},
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

func Test_BuildStepGraph_ChainsPathDependencies(t *testing.T) {
	// A -> B -> C chain via paths
	steps := []spec.Step{
		&mockResourceStep{desc: "A", provides: paths("/a")},
		&mockResourceStep{desc: "B", requires: paths("/a"), provides: paths("/b")},
		&mockResourceStep{desc: "C", requires: paths("/b"), provides: paths("/c")},
	}

	nodes := buildStepGraph(steps)

	noDeps(t, nodes[0], "A")
	requiresExactly(t, nodes[1], "B", nodes[0])
	requiresExactly(t, nodes[2], "C", nodes[1])
}

func Test_BuildStepGraph_OrdersChildPathAfterParentDir(t *testing.T) {
	// dir creates /home/user/.ssh, copy writes /home/user/.ssh/authorized_keys
	// -> copy should depend on dir (parent directory)
	steps := []spec.Step{
		&mockResourceStep{desc: "dir", provides: paths("/home/user/.ssh")},
		&mockResourceStep{
			desc: "copy", requires: paths("./keys"),
			provides: paths("/home/user/.ssh/authorized_keys"),
		},
	}

	nodes := buildStepGraph(steps)

	requiresExactly(t, nodes[1], "copy", nodes[0])
}

func Test_BuildStepGraph_OrdersUserConsumerAfterProducer(t *testing.T) {
	// user step provides user "app", dir step requires user "app" -> dependency
	steps := []spec.Step{
		&mockResourceStep{desc: "user", provides: users("app")},
		&mockResourceStep{desc: "dir", requires: users("app"), provides: paths("/opt/app")},
	}

	nodes := buildStepGraph(steps)
	requiresExactly(t, nodes[1], "dir", nodes[0])
}

func Test_BuildStepGraph_OrdersGroupConsumerAfterProducer(t *testing.T) {
	// group step provides group "staff", dir step requires group "staff" -> dependency
	steps := []spec.Step{
		&mockResourceStep{desc: "group", provides: groups("staff")},
		&mockResourceStep{desc: "dir", requires: groups("staff"), provides: paths("/srv")},
	}

	nodes := buildStepGraph(steps)
	requiresExactly(t, nodes[1], "dir", nodes[0])
}

func Test_BuildStepGraph_IgnoresCrossKindNameOverlap(t *testing.T) {
	// A provides path "/foo", B requires user "foo" -> no dependency (different kinds)
	steps := []spec.Step{
		&mockResourceStep{desc: "A", provides: paths("/foo")},
		&mockResourceStep{desc: "B", requires: users("foo"), provides: paths("/bar")},
	}

	nodes := buildStepGraph(steps)
	noDeps(t, nodes[1], "B")
}

func Test_BuildStepGraph_DoesNotFenceUserProvider(t *testing.T) {
	// A user step with resources is NOT a barrier - parallel path steps
	// should not be serialized through it.
	// P1, user, P2 with no resource overlap: P1 and P2 run in parallel,
	// user is not a barrier because it has resources (user resource).
	steps := []spec.Step{
		&mockResourceStep{desc: "P1", provides: paths("/a")},
		&mockResourceStep{desc: "user", provides: users("app")},
		&mockResourceStep{desc: "P2", provides: paths("/b")},
	}

	nodes := buildStepGraph(steps)

	noDeps(t, nodes[0], "P1")
	noDeps(t, nodes[1], "user")
	noDeps(t, nodes[2], "P2")
}

func Test_BuildStepGraph_ChainsMixedResourceKinds(t *testing.T) {
	// group -> user (requires group) -> dir (requires user and path)
	steps := []spec.Step{
		&mockResourceStep{desc: "group", provides: groups("staff")},
		&mockResourceStep{desc: "user", requires: groups("staff"), provides: users("app")},
		&mockResourceStep{desc: "dir", requires: users("app"), provides: paths("/opt/app")},
	}

	nodes := buildStepGraph(steps)

	requiresExactly(t, nodes[1], "user", nodes[0])
	requiresExactly(t, nodes[2], "dir", nodes[1])
}

func Test_BuildStepGraph_ParallelizesDistinctLabels(t *testing.T) {
	// Three steps with distinct label slots - no resource overlap
	// and not barriers, so they run in parallel.
	steps := []spec.Step{
		&mockResourceStep{desc: "node100", provides: labels("node:100")},
		&mockResourceStep{desc: "node101", provides: labels("node:101")},
		&mockResourceStep{desc: "node102", provides: labels("node:102")},
	}

	nodes := buildStepGraph(steps)

	for _, n := range nodes {
		noDeps(t, n, n.step.Desc())
	}
}

func Test_BuildStepGraph_DoesNotFenceLabelProvider(t *testing.T) {
	// A label-resource step between two path steps must not act
	// as a barrier (regression test for #235).
	steps := []spec.Step{
		&mockResourceStep{desc: "P1", provides: paths("/a")},
		&mockResourceStep{desc: "node", provides: labels("node:100")},
		&mockResourceStep{desc: "P2", provides: paths("/b")},
	}

	nodes := buildStepGraph(steps)

	for _, n := range nodes {
		noDeps(t, n, n.step.Desc())
	}
}

func Test_BuildStepGraph_LimitsParentDirMatchingToPaths(t *testing.T) {
	// Parent-directory prefix matching only applies to path resources.
	// user "app" should NOT create a dependency on user "app/sub".
	steps := []spec.Step{
		&mockResourceStep{desc: "A", provides: users("app")},
		&mockResourceStep{desc: "B", provides: users("app/sub")},
	}

	nodes := buildStepGraph(steps)
	noDeps(t, nodes[1], "B")
}

func Test_InitStepPending_CountsDeps(t *testing.T) {
	steps := []spec.Step{
		&mockResourceStep{desc: "A", provides: paths("/foo")},
		&mockResourceStep{desc: "B", requires: paths("/foo")},
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

// Opaque-step barrier guarantee
// -----------------------------------------------------------------------------
//
// posix.run, posix.pkg, and posix.service emit steps whose side
// effects scampi cannot statically reason about: a `run` is arbitrary
// shell, a `pkg` install can drop files anywhere on disk, a `service`
// start may rewrite `/etc` via post-start units. Every one of these
// has to fence - concurrent execution against any sibling step is
// unsafe.
//
// The mechanism today: such steps don't implement spec.Provider
// (or implement it trivially) and `hasResources` reports false, which
// makes `buildStepGraph` chain them as barriers. These tests pin
// that contract: regressing the barrier is silent in the engine -
// nothing crashes, you just race.

// asStep plans a step instance (StepID is irrelevant for graph
// shape - the engine only cares about Type/Config) and returns the
// resulting step.
func asStep(t *testing.T, st spec.StepKind, cfg any) spec.Step {
	t.Helper()
	act, err := st.Plan(spec.DeclaredStep{Type: st, Config: cfg})
	if err != nil {
		t.Fatalf("%s.Plan: %v", st.Kind(), err)
	}
	return act
}

func Test_RunStep_IsBarrier(t *testing.T) {
	act := asStep(t, run.Run{}, &run.RunConfig{
		Apply: "echo hi",
		Check: "true",
	})
	if hasResources(act) {
		// posix.run is arbitrary shell - concurrent execution alongside
		// any other step is unsafe.
		t.Fatal("posix.run step must be a barrier")
	}
}

func Test_PkgStep_IsBarrier(t *testing.T) {
	act := asStep(t, pkg.Pkg{}, &pkg.PkgConfig{
		Packages: []string{"vim"},
		Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
		State:    "present",
	})
	if hasResources(act) {
		// pkg installs can drop files anywhere and run arbitrary
		// post-install hooks.
		t.Fatal("posix.pkg step must be a barrier")
	}
}

func Test_ServiceStep_IsBarrier(t *testing.T) {
	act := asStep(t, service.Service{}, &service.ServiceConfig{
		Name:    "samba-ad-dc",
		State:   "running",
		Enabled: true,
	})
	if hasResources(act) {
		// service start hooks can rewrite /etc, spawn child units, etc.
		t.Fatal("posix.service step must be a barrier")
	}
}

func Test_BuildStepGraph_SerializesPkgServiceRun(t *testing.T) {
	// dc1-v2-shaped sequence: pkg -> service -> run -> run -> run -> service.
	// Every step is opaque (barrier), so the fence builder must chain
	// them strictly: each step depends on the immediately preceding
	// one. If anyone adds Provider/Requirer to one of these step types without
	// an exact, complete declaration, this test fails.
	steps := []spec.Step{
		asStep(t, pkg.Pkg{}, &pkg.PkgConfig{
			Packages: []string{"samba"},
			Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
			State:    "present",
		}),
		asStep(t, service.Service{}, &service.ServiceConfig{
			Name: "smbd", State: "stopped", Enabled: false,
		}),
		asStep(t, run.Run{}, &run.RunConfig{
			Apply: "samba-tool domain provision ...", Check: "test -f /var/lib/samba/private/sam.ldb",
		}),
		asStep(t, run.Run{}, &run.RunConfig{
			Apply: "install -m 0644 /var/lib/samba/private/krb5.conf /etc/krb5.conf",
			Check: "cmp -s /var/lib/samba/private/krb5.conf /etc/krb5.conf",
		}),
		asStep(t, service.Service{}, &service.ServiceConfig{
			Name: "samba-ad-dc", State: "running", Enabled: true,
		}),
	}
	nodes := buildStepGraph(steps)

	// Every node except the first depends on the previous one; the
	// chain is linear because every step is a barrier.
	for i := 1; i < len(nodes); i++ {
		requiresExactly(t, nodes[i], nodes[i].step.Kind(), nodes[i-1])
	}
}

func Test_BuildStepGraph_FencesBarrierBetweenPathers(t *testing.T) {
	// posix.copy declares a path resource (it's a Pather, NOT a barrier).
	// A run between two copies must still fence - the run can read or
	// write anything, including files copy is touching.
	steps := []spec.Step{
		asStep(t, copy.Copy{}, &copy.CopyConfig{
			Dest:  "/etc/krb5.conf",
			Perm:  "0644",
			Owner: "root",
			Group: "root",
		}),
		asStep(t, run.Run{}, &run.RunConfig{
			Apply: "samba-tool ...", Check: "test -f /var/lib/samba/private/sam.ldb",
		}),
		asStep(t, copy.Copy{}, &copy.CopyConfig{
			Dest:  "/etc/samba/smb.conf",
			Perm:  "0644",
			Owner: "root",
			Group: "root",
		}),
	}
	nodes := buildStepGraph(steps)

	// run (idx 1) must depend on the prior copy.
	requiresExactly(t, nodes[1], "run", nodes[0])
	// the second copy (idx 2) must depend on the run barrier.
	requiresExactly(t, nodes[2], "copy", nodes[1])
}
