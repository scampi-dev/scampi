// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"testing"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// recordingEmitter captures EmitGraph calls for assertion. Other Emit*
// methods are no-ops — the suite only exercises graph emission.
type recordingEmitter struct {
	noopEmitter
	graphs []event.GraphEvent
}

func (r *recordingEmitter) EmitGraph(e event.GraphEvent) {
	r.graphs = append(r.graphs, e)
}

func TestEmitGraphSummary_SuppressesTrivial(t *testing.T) {
	// Single deploy, no deps → no graph render. Avoids noise on
	// configs with one block and on intra-deploy work.
	a := mkResolved("a", fakeTargetType{kind: "ssh"})
	graph, err := buildDeployGraph([]spec.ResolvedConfig{a})
	if err != nil {
		t.Fatalf("buildDeployGraph: %v", err)
	}

	em := &recordingEmitter{}
	emitGraphSummary(em, graph)
	if len(em.graphs) != 0 {
		t.Errorf("expected suppressed (trivial graph), got %d events", len(em.graphs))
	}
}

func TestEmitGraphSummary_RendersOrderedTopology(t *testing.T) {
	// dc-lxc → dc1-v2 chain via lxc:1000.
	create := mkResolved(
		"create",
		fakeTargetType{kind: "ssh"},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(1000)}},
	)
	configure := mkResolved(
		"configure",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(1000)}},
	)

	graph, err := buildDeployGraph([]spec.ResolvedConfig{create, configure})
	if err != nil {
		t.Fatalf("buildDeployGraph: %v", err)
	}

	em := &recordingEmitter{}
	emitGraphSummary(em, graph)

	if len(em.graphs) != 1 {
		t.Fatalf("expected 1 graph event, got %d", len(em.graphs))
	}
	g := em.graphs[0]
	if len(g.Detail.Levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(g.Detail.Levels))
	}
	root := g.Detail.Levels[0].Nodes
	if len(root) != 1 || root[0].DeployName != "create" {
		t.Errorf("level 0 = %+v, want [create]", root)
	}
	dependent := g.Detail.Levels[1].Nodes
	if len(dependent) != 1 || dependent[0].DeployName != "configure" {
		t.Errorf("level 1 = %+v, want [configure]", dependent)
	}
	if len(dependent[0].After) != 1 || dependent[0].After[0] != "create" {
		t.Errorf("After = %+v, want [create]", dependent[0].After)
	}
	if len(dependent[0].Needs) != 1 || dependent[0].Needs[0] != "lxc:1000" {
		t.Errorf("Needs = %+v, want [lxc:1000]", dependent[0].Needs)
	}
}

func TestEmitGraphSummary_HandlesNilEmitter(_ *testing.T) {
	// Defensive: callers might pass nil em in tests / special paths.
	graph, _ := buildDeployGraph([]spec.ResolvedConfig{
		mkResolved("a", fakeTargetType{kind: "ssh"}),
	})
	emitGraphSummary(nil, graph) // must not panic
}
