// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// noopEmitter
// -----------------------------------------------------------------------------

type noopEmitter struct{}

func (noopEmitter) EmitEngineLifecycle(event.EngineEvent)       {}
func (noopEmitter) EmitPlanLifecycle(event.PlanEvent)           {}
func (noopEmitter) EmitActionLifecycle(event.ActionEvent)       {}
func (noopEmitter) EmitOpLifecycle(event.OpEvent)               {}
func (noopEmitter) EmitIndexAll(event.IndexAllEvent)            {}
func (noopEmitter) EmitIndexStep(event.IndexStepEvent)          {}
func (noopEmitter) EmitEngineDiagnostic(event.EngineDiagnostic) {}
func (noopEmitter) EmitPlanDiagnostic(event.PlanDiagnostic)     {}
func (noopEmitter) EmitActionDiagnostic(event.ActionDiagnostic) {}
func (noopEmitter) EmitOpDiagnostic(event.OpDiagnostic)         {}

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
		Unit: spec.Unit{
			Actions: []spec.Action{act},
		},
	}

	err := DetectPlanCycles(noopEmitter{}, plan)
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
		Unit: spec.Unit{
			Actions: []spec.Action{act},
		},
	}

	err := DetectPlanCycles(noopEmitter{}, plan)
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
	err := DetectPlanCycles(noopEmitter{}, plan)
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
	err := detectHookCycles(noopEmitter{}, hooks)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestDetectHookCycles_SimpleCycle(t *testing.T) {
	hooks := map[string][]spec.StepInstance{
		"a": {{OnChange: []string{"b"}, Fields: map[string]spec.FieldSpan{}}},
		"b": {{OnChange: []string{"a"}, Fields: map[string]spec.FieldSpan{}}},
	}
	err := detectHookCycles(noopEmitter{}, hooks)
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
	err := detectHookCycles(noopEmitter{}, hooks)
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
	err := detectHookCycles(noopEmitter{}, nil)
	if err != nil {
		t.Errorf("expected no error for nil hooks, got %v", err)
	}

	err = detectHookCycles(noopEmitter{}, map[string][]spec.StepInstance{})
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
	err := detectHookCycles(noopEmitter{}, hooks)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}
