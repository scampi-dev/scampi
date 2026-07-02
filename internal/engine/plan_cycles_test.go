// SPDX-License-Identifier: GPL-3.0-only

// Scope: cross-cutting tests for cycle detection inside the planning
// pipeline (full LoadConfig + Plan flow, not unit-level graph code).
// Exercises: plan.go (Plan, planSteps), step_graph.go (graph build
// from before+after links), and the surfacing of cycles as
// engine.CycleError to callers. Tests use real scampi configs that
// declare cyclic before/after relations.

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

// discardCtx returns a Ctx whose emitter drops everything, for tests that
// exercise planning (and its diagnostics) without inspecting them.
func discardCtx(t *testing.T) diagnostic.Ctx {
	return diagnostic.NewCtx(t.Context(), diagnostic.NewEmitter(diagnostic.Policy{}, diagnostic.Discard{}))
}

// mockOp
// -----------------------------------------------------------------------------

type mockOp struct {
	step spec.Step
	deps []spec.Op
}

func (o *mockOp) Step() spec.Step                             { return o.step }
func (o *mockOp) DependsOn() []spec.Op                        { return o.deps }
func (o *mockOp) RequiredCapabilities() capability.Capability { return 0 }

func (o *mockOp) Check(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
	return spec.CheckSatisfied, nil, nil
}

func (o *mockOp) Execute(context.Context, source.Source, target.Target) (spec.Result, error) {
	return spec.Result{}, nil
}

// Plan cycle tests
// -----------------------------------------------------------------------------

func Test_DetectPlanCycles_NoCycle(t *testing.T) {
	act := &mockStep{desc: "test", kind: "test"}
	opA := &mockOp{step: act}
	opB := &mockOp{step: act, deps: []spec.Op{opA}}
	act.ops = []spec.Op{opA, opB}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			Steps: []spec.Step{act},
		},
	}

	err := DetectPlanCycles(discardCtx(t), plan)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func Test_DetectPlanCycles_SimpleCycle(t *testing.T) {
	act := &mockStep{desc: "test", kind: "test"}
	opA := &mockOp{step: act}
	opB := &mockOp{step: act}
	opA.deps = []spec.Op{opB}
	opB.deps = []spec.Op{opA}
	act.ops = []spec.Op{opA, opB}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			Steps: []spec.Step{act},
		},
	}

	err := DetectPlanCycles(discardCtx(t), plan)
	if err == nil {
		t.Fatal("expected cycle error")
	}

	var abort AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T", err)
	}
	if len(abort.Causes) == 0 {
		t.Fatal("expected at least one cause")
	}

	var cycleErr CyclicDependencyError
	if !errors.As(abort.Causes[0], &cycleErr) {
		t.Fatalf("expected CyclicDependencyError, got %T", abort.Causes[0])
	}
}

func Test_DetectPlanCycles_NoSteps(t *testing.T) {
	plan := spec.Plan{}
	err := DetectPlanCycles(discardCtx(t), plan)
	if err != nil {
		t.Errorf("expected no error for empty plan, got %v", err)
	}
}

// Hook cycle tests
// -----------------------------------------------------------------------------

func Test_DetectHookCycles_NoCycle(t *testing.T) {
	hooks := map[string][]spec.DeclaredStep{
		"a": {{OnChange: []string{"b"}}},
		"b": {},
	}
	err := detectHookCycles(discardCtx(t), hooks)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func Test_DetectHookCycles_SimpleCycle(t *testing.T) {
	hooks := map[string][]spec.DeclaredStep{
		"a": {{OnChange: []string{"b"}, Fields: map[string]spec.FieldSpan{}}},
		"b": {{OnChange: []string{"a"}, Fields: map[string]spec.FieldSpan{}}},
	}
	err := detectHookCycles(discardCtx(t), hooks)
	if err == nil {
		t.Fatal("expected cycle error")
	}

	var abort AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T", err)
	}

	var hookErr HookCycleError
	if !errors.As(abort.Causes[0], &hookErr) {
		t.Fatalf("expected HookCycleError, got %T", abort.Causes[0])
	}
	if len(hookErr.Chain) < 3 {
		t.Errorf("expected cycle chain of at least 3, got %d", len(hookErr.Chain))
	}
}

func Test_DetectHookCycles_SelfCycle(t *testing.T) {
	hooks := map[string][]spec.DeclaredStep{
		"a": {{OnChange: []string{"a"}, Fields: map[string]spec.FieldSpan{}}},
	}
	err := detectHookCycles(discardCtx(t), hooks)
	if err == nil {
		t.Fatal("expected cycle error")
	}

	var abort AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T", err)
	}

	var hookErr HookCycleError
	if !errors.As(abort.Causes[0], &hookErr) {
		t.Fatalf("expected HookCycleError, got %T", abort.Causes[0])
	}

	chain := strings.Join(hookErr.Chain, " -> ")
	if !strings.Contains(chain, "a") {
		t.Errorf("expected chain to contain 'a', got %q", chain)
	}
}

func Test_DetectHookCycles_Empty(t *testing.T) {
	err := detectHookCycles(discardCtx(t), nil)
	if err != nil {
		t.Errorf("expected no error for nil hooks, got %v", err)
	}

	err = detectHookCycles(discardCtx(t), map[string][]spec.DeclaredStep{})
	if err != nil {
		t.Errorf("expected no error for empty hooks, got %v", err)
	}
}

func Test_DetectHookCycles_ThreeNodeCycle(t *testing.T) {
	hooks := map[string][]spec.DeclaredStep{
		"a": {{OnChange: []string{"b"}, Fields: map[string]spec.FieldSpan{}}},
		"b": {{OnChange: []string{"c"}, Fields: map[string]spec.FieldSpan{}}},
		"c": {{OnChange: []string{"a"}, Fields: map[string]spec.FieldSpan{}}},
	}
	err := detectHookCycles(discardCtx(t), hooks)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

// Step cycle tests
// -----------------------------------------------------------------------------
//
// Step-level cycle detection: a planned step DAG with introduced
// cycles is rejected with an error whose payload names every node
// along the cycle in order.

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

func Test_DetectStepCycles_NoCycle(t *testing.T) {
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

func Test_DetectStepCycles_SimpleCycle(t *testing.T) {
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

func Test_DetectStepCycles_IndependentSteps(t *testing.T) {
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

func Test_StepCyclicDependency_Error(t *testing.T) {
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
