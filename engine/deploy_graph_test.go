// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"testing"

	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// fakeStaticStepType implements spec.StepType + StaticPromiseProvider.
type fakeStaticStepType struct {
	kind     string
	promises []spec.Resource
}

func (f fakeStaticStepType) Kind() string                                  { return f.kind }
func (f fakeStaticStepType) NewConfig() any                                { return &struct{}{} }
func (f fakeStaticStepType) Plan(_ spec.StepInstance) (spec.Action, error) { return nil, nil }
func (f fakeStaticStepType) StaticPromises(_ any) []spec.Resource          { return f.promises }

// fakeLabelConfig implements spec.ResourceDeclarer for testing user-driven
// promises/inputs declared on step Configs (e.g. posix.run, posix.service).
type fakeLabelConfig struct {
	promises []string
	inputs   []string
}

func (c *fakeLabelConfig) ResourceDeclarations() ([]string, []string) {
	return c.promises, c.inputs
}

// fakeLabelStepType is a step type whose Config implements ResourceDeclarer.
// Used to verify label-based ordering across deploy blocks.
type fakeLabelStepType struct{ kind string }

func (f fakeLabelStepType) Kind() string                                  { return f.kind }
func (f fakeLabelStepType) NewConfig() any                                { return &fakeLabelConfig{} }
func (f fakeLabelStepType) Plan(_ spec.StepInstance) (spec.Action, error) { return nil, nil }

func mkLabeledStep(promises, inputs []string) spec.StepInstance {
	return spec.StepInstance{
		Type:   fakeLabelStepType{kind: "label"},
		Config: &fakeLabelConfig{promises: promises, inputs: inputs},
	}
}

// fakeTargetType implements spec.TargetType + StaticInputProvider.
type fakeTargetType struct {
	kind   string
	inputs []spec.Resource
}

func (f fakeTargetType) Kind() string   { return f.kind }
func (f fakeTargetType) NewConfig() any { return &struct{}{} }
func (f fakeTargetType) StaticInputs(_ any) []spec.Resource {
	return f.inputs
}
func (f fakeTargetType) Create(_ context.Context, _ source.Source, _ spec.TargetInstance) (target.Target, error) {
	return nil, nil
}

func mkResolved(name string, target spec.TargetType, steps ...spec.StepType) spec.ResolvedConfig {
	stepInsts := make([]spec.StepInstance, len(steps))
	for i, s := range steps {
		stepInsts[i] = spec.StepInstance{Type: s}
	}
	return spec.ResolvedConfig{
		DeployName: name,
		TargetName: name,
		Target:     spec.TargetInstance{Type: target},
		Steps:      stepInsts,
	}
}

func TestBuildDeployGraphSingleProducer(t *testing.T) {
	create := mkResolved(
		"create",
		fakeTargetType{kind: "ssh"},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(1000)}},
	)
	configure := mkResolved(
		"configure",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(1000)}},
	)

	g, err := buildDeployGraph([]spec.ResolvedConfig{create, configure})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(g.levels))
	}
	if g.levels[0][0].res.DeployName != "create" {
		t.Errorf("level 0 should be 'create', got %q", g.levels[0][0].res.DeployName)
	}
	if g.levels[1][0].res.DeployName != "configure" {
		t.Errorf("level 1 should be 'configure', got %q", g.levels[1][0].res.DeployName)
	}
}

func TestBuildDeployGraphExternalInput(t *testing.T) {
	// Configure-only: nobody in this run produces lxc:1000.
	configure := mkResolved("configure",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(1000)}},
	)
	g, err := buildDeployGraph([]spec.ResolvedConfig{configure})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.levels) != 1 {
		t.Fatalf("expected single level (external input), got %d", len(g.levels))
	}
	if len(g.levels[0]) != 1 {
		t.Errorf("expected 1 node at level 0, got %d", len(g.levels[0]))
	}
}

func TestBuildDeployGraphIndependentParallel(t *testing.T) {
	// Two unrelated deploys — no resource flow → both at level 0.
	a := mkResolved("a", fakeTargetType{kind: "ssh"})
	b := mkResolved("b", fakeTargetType{kind: "rest"})

	g, err := buildDeployGraph([]spec.ResolvedConfig{a, b})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.levels) != 1 {
		t.Fatalf("expected 1 level (independent), got %d", len(g.levels))
	}
	if len(g.levels[0]) != 2 {
		t.Errorf("expected both nodes at level 0, got %d", len(g.levels[0]))
	}
}

func TestBuildDeployGraphMultipleProducers(t *testing.T) {
	a := mkResolved(
		"a",
		fakeTargetType{kind: "ssh"},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(1000)}},
	)
	b := mkResolved(
		"b",
		fakeTargetType{kind: "ssh"},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(1000)}},
	)
	_, err := buildDeployGraph([]spec.ResolvedConfig{a, b})
	var multi MultipleProducersError
	if !errors.As(err, &multi) {
		t.Fatalf("expected MultipleProducersError, got %T: %v", err, err)
	}
	if multi.Resource.Name != "1000" {
		t.Errorf("Resource.Name = %q, want %q", multi.Resource.Name, "1000")
	}
}

func TestBuildDeployGraphCycle(t *testing.T) {
	// a produces lxc:1000, consumes lxc:2000
	// b produces lxc:2000, consumes lxc:1000
	// → cycle.
	a := mkResolved("a",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(2000)}},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(1000)}},
	)
	b := mkResolved("b",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(1000)}},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(2000)}},
	)
	_, err := buildDeployGraph([]spec.ResolvedConfig{a, b})
	var cycle DeployCycleError
	if !errors.As(err, &cycle) {
		t.Fatalf("expected DeployCycleError, got %T: %v", err, err)
	}
}

func TestBuildDeployGraphChain(t *testing.T) {
	// a → b → c, three levels.
	a := mkResolved(
		"a",
		fakeTargetType{kind: "ssh"},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(1000)}},
	)
	b := mkResolved("b",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(1000)}},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(2000)}},
	)
	c := mkResolved("c",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(2000)}},
	)

	g, err := buildDeployGraph([]spec.ResolvedConfig{a, b, c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(g.levels))
	}
	for i, want := range []string{"a", "b", "c"} {
		if g.levels[i][0].res.DeployName != want {
			t.Errorf("level %d should be %q, got %q", i, want, g.levels[i][0].res.DeployName)
		}
	}
}

func TestBuildDeployGraphLabelOrdering(t *testing.T) {
	// dc1 promises "realm:skrynet.lan" via a step config; dc2 inputs
	// it. Engine orders dc2 after dc1.
	dc1 := spec.ResolvedConfig{
		DeployName: "dc1",
		TargetName: "dc1",
		Target:     spec.TargetInstance{Type: fakeTargetType{kind: "ssh"}},
		Steps: []spec.StepInstance{
			mkLabeledStep([]string{"realm:skrynet.lan"}, nil),
		},
	}
	dc2 := spec.ResolvedConfig{
		DeployName: "dc2",
		TargetName: "dc2",
		Target:     spec.TargetInstance{Type: fakeTargetType{kind: "ssh"}},
		Steps: []spec.StepInstance{
			mkLabeledStep(nil, []string{"realm:skrynet.lan"}),
		},
	}
	g, err := buildDeployGraph([]spec.ResolvedConfig{dc1, dc2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(g.levels))
	}
	if g.levels[0][0].res.DeployName != "dc1" {
		t.Errorf("level 0 should be dc1, got %q", g.levels[0][0].res.DeployName)
	}
	if g.levels[1][0].res.DeployName != "dc2" {
		t.Errorf("level 1 should be dc2, got %q", g.levels[1][0].res.DeployName)
	}
}

func TestBuildDeployGraphLabelExternalInput(t *testing.T) {
	// Consumer-only: no producer of "realm:skrynet.lan" in this run.
	// Treated as external — runs immediately as a root.
	dc2 := spec.ResolvedConfig{
		DeployName: "dc2",
		TargetName: "dc2",
		Target:     spec.TargetInstance{Type: fakeTargetType{kind: "ssh"}},
		Steps: []spec.StepInstance{
			mkLabeledStep(nil, []string{"realm:skrynet.lan"}),
		},
	}
	g, err := buildDeployGraph([]spec.ResolvedConfig{dc2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.levels) != 1 || len(g.levels[0]) != 1 {
		t.Errorf("expected single root, got levels=%v", levelSizes(g.levels))
	}
}

func levelSizes(levels [][]*deployNode) []int {
	sizes := make([]int, len(levels))
	for i, l := range levels {
		sizes[i] = len(l)
	}
	return sizes
}

func TestBuildDeployGraphFanout(t *testing.T) {
	// a produces lxc:1000 + lxc:1001, b consumes :1000, c consumes :1001
	// → b and c run in parallel at level 1.
	a := mkResolved(
		"a",
		fakeTargetType{kind: "ssh"},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(1000)}},
		fakeStaticStepType{kind: "pve.lxc", promises: []spec.Resource{spec.LXCResource(1001)}},
	)
	b := mkResolved("b",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(1000)}},
	)
	c := mkResolved("c",
		fakeTargetType{kind: "pve.lxc_target", inputs: []spec.Resource{spec.LXCResource(1001)}},
	)

	g, err := buildDeployGraph([]spec.ResolvedConfig{a, b, c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(g.levels))
	}
	if len(g.levels[1]) != 2 {
		t.Errorf("expected 2 nodes at level 1 (fanout parallel), got %d", len(g.levels[1]))
	}
}
