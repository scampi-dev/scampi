// SPDX-License-Identifier: GPL-3.0-only

// Scope: cross-cutting tests for cycle detection inside the planning
// pipeline (full LoadConfig + Plan flow, not unit-level graph code).
// Exercises: plan.go (Plan, planSteps), action_graph.go (graph build
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
	action spec.Action
	deps   []spec.Op
}

func (o *mockOp) Action() spec.Action                         { return o.action }
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

func TestDetectPlanCycles_NoCycle(t *testing.T) {
	act := &mockAction{desc: "test", kind: "test"}
	opA := &mockOp{action: act}
	opB := &mockOp{action: act, deps: []spec.Op{opA}}
	act.ops = []spec.Op{opA, opB}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			Actions: []spec.Action{act},
		},
	}

	err := DetectPlanCycles(discardCtx(t), plan)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestDetectPlanCycles_SimpleCycle(t *testing.T) {
	act := &mockAction{desc: "test", kind: "test"}
	opA := &mockOp{action: act}
	opB := &mockOp{action: act}
	opA.deps = []spec.Op{opB}
	opB.deps = []spec.Op{opA}
	act.ops = []spec.Op{opA, opB}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			Actions: []spec.Action{act},
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

func TestDetectPlanCycles_NoActions(t *testing.T) {
	plan := spec.Plan{}
	err := DetectPlanCycles(discardCtx(t), plan)
	if err != nil {
		t.Errorf("expected no error for empty plan, got %v", err)
	}
}

// Hook cycle tests
// -----------------------------------------------------------------------------

func TestDetectHookCycles_NoCycle(t *testing.T) {
	hooks := map[string][]spec.StepInstance{
		"a": {{OnChange: []string{"b"}}},
		"b": {},
	}
	err := detectHookCycles(discardCtx(t), hooks)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestDetectHookCycles_SimpleCycle(t *testing.T) {
	hooks := map[string][]spec.StepInstance{
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

func TestDetectHookCycles_SelfCycle(t *testing.T) {
	hooks := map[string][]spec.StepInstance{
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

func TestDetectHookCycles_Empty(t *testing.T) {
	err := detectHookCycles(discardCtx(t), nil)
	if err != nil {
		t.Errorf("expected no error for nil hooks, got %v", err)
	}

	err = detectHookCycles(discardCtx(t), map[string][]spec.StepInstance{})
	if err != nil {
		t.Errorf("expected no error for empty hooks, got %v", err)
	}
}

func TestDetectHookCycles_ThreeNodeCycle(t *testing.T) {
	hooks := map[string][]spec.StepInstance{
		"a": {{OnChange: []string{"b"}, Fields: map[string]spec.FieldSpan{}}},
		"b": {{OnChange: []string{"c"}, Fields: map[string]spec.FieldSpan{}}},
		"c": {{OnChange: []string{"a"}, Fields: map[string]spec.FieldSpan{}}},
	}
	err := detectHookCycles(discardCtx(t), hooks)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}
