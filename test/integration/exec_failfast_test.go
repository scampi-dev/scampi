// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/model"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target/local"
	"scampi.dev/scampi/test/harness"
)

// Dependency graph:
//
//	A   B   C
//
// (no dependencies)
//
// All ops:
//
//	Check = Satisfied
//	Execute = MUST NOT be called
func TestExecuteAction_AllOpsSkipped(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.PanicExecFn("A.Execute must not be called"),
	}
	opB := &harness.FakeOp{
		Name:    "B",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.PanicExecFn("B.Execute must not be called"),
	}
	opC := &harness.FakeOp{
		Name:    "C",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.PanicExecFn("C.Execute must not be called"),
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA, opB, opC),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(ctx, plan)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	if opA.CheckCalls != 1 || opB.CheckCalls != 1 || opC.CheckCalls != 1 {
		t.Fatalf("expected all checks to run once")
	}

	ar := mustSingleAction(t, rep)
	mustOpOutcome(t, ar, "A", model.OpSkipped)
	mustOpOutcome(t, ar, "B", model.OpSkipped)
	mustOpOutcome(t, ar, "C", model.OpSkipped)
}

// Dependency graph:
//
//	A → B → C
//
// All ops:
//
//	Check = Unsatisfied
//	Execute = Success
func TestExecuteAction_LinearSuccess(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.OkExecFn(true),
	}
	opB := &harness.FakeOp{
		Name:    "B",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.OkExecFn(false),
	}
	opC := &harness.FakeOp{
		Name:    "C",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.OkExecFn(true),
	}

	opB.Deps = []spec.Op{opA}
	opC.Deps = []spec.Op{opB}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA, opB, opC),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(ctx, plan)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	if opA.ExecCalls != 1 || opB.ExecCalls != 1 || opC.ExecCalls != 1 {
		t.Fatalf("expected all ops to execute once")
	}

	ar := mustSingleAction(t, rep)
	mustOpOutcome(t, ar, "A", model.OpSucceeded)
	mustOpOutcome(t, ar, "B", model.OpSucceeded)
	mustOpOutcome(t, ar, "C", model.OpSucceeded)
}

// Dependency graph:
//
//	A → B → C
//
// Behavior:
//
//	A.Execute → Success
//	B.Execute → Abort
//	C.Execute → MUST NOT be called
func TestExecuteAction_FailFast_MiddleOfChain(t *testing.T) {
	var act *harness.FakeAction

	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.OkExecFn(true),
	}

	opB := &harness.FakeOp{
		Name:    "B",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.DiagExecFn(signal.Error, diagnostic.ImpactAbort),
	}

	opC := &harness.FakeOp{
		Name:    "C",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("C.Execute must not be called"),
	}

	opB.Deps = []spec.Op{opA}
	opC.Deps = []spec.Op{opB}

	act = harness.MkAction(opA, opB, opC)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				act,
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(ctx, plan)
	if err == nil {
		t.Fatalf("expected execution error, got nil")
	}

	if opA.CheckCalls != 1 || opB.CheckCalls != 1 || opC.CheckCalls != 1 {
		t.Fatalf("expected Check() to be called once for all ops, got A=%d B=%d C=%d",
			opA.CheckCalls, opB.CheckCalls, opC.CheckCalls)
	}

	if opA.ExecCalls != 1 {
		t.Fatalf("expected A to execute once, got %d", opA.ExecCalls)
	}
	if opB.ExecCalls != 1 {
		t.Fatalf("expected B to execute once, got %d", opB.ExecCalls)
	}
	if opC.ExecCalls != 0 {
		t.Fatalf("expected C not to execute, got %d", opC.ExecCalls)
	}

	ar := mustSingleAction(t, rep)
	mustOpOutcome(t, ar, "A", model.OpSucceeded)
	mustOpOutcome(t, ar, "B", model.OpFailed)
	mustOpOutcome(t, ar, "C", model.OpAborted)
}

// Dependency graph:
//
//	  A
//	 / \
//	B   C
//	     \
//	      D
//
// Behavior:
//
//	A.Execute → Success
//	B.Execute → Success
//	C.Execute → Abort
//	D.Execute → MUST NOT be called
func TestExecuteAction_BranchFailure(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.OkExecFn(true),
	}
	opB := &harness.FakeOp{
		Name:    "B",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.OkExecFn(true),
	}
	opC := &harness.FakeOp{
		Name:    "C",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.DiagExecFn(signal.Error, diagnostic.ImpactAbort),
	}
	opD := &harness.FakeOp{
		Name:    "D",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("D.Execute must not be called"),
	}

	opB.Deps = []spec.Op{opA}
	opC.Deps = []spec.Op{opA}
	opD.Deps = []spec.Op{opC}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA, opB, opC, opD),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(ctx, plan)
	if err == nil {
		t.Fatalf("expected execution error")
	}

	if opB.ExecCalls != 1 {
		t.Fatalf("expected B to execute")
	}
	if opD.ExecCalls != 0 {
		t.Fatalf("expected D not to execute")
	}

	ar := mustSingleAction(t, rep)
	mustOpOutcome(t, ar, "A", model.OpSucceeded)
	mustOpOutcome(t, ar, "B", model.OpSucceeded)
	mustOpOutcome(t, ar, "C", model.OpFailed)
	mustOpOutcome(t, ar, "D", model.OpAborted)
}

// Dependency graph:
//
//	A
//
// Behavior:
//
//	A.Check   → Diagnostic (Warning, Continue)
//	A.Execute → Success
func TestExecuteAction_CheckDiagnostic_Continues(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.DiagCheckFn(signal.Warning, diagnostic.ImpactNone),
		ExecFn:  harness.OkExecFn(true),
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(ctx, plan)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	if opA.ExecCalls != 1 {
		t.Fatalf("expected A to execute")
	}

	ar := mustSingleAction(t, rep)
	mustOpOutcome(t, ar, "A", model.OpSucceeded)
}

// Dependency graph:
//
//	A → B
//
// Behavior:
//
//	A.Check   → Diagnostic (Abort)
//	A.Execute → MUST NOT be called
//	B.Execute → MUST NOT be called
func TestExecuteAction_AbortDuringCheck(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.DiagCheckFn(signal.Error, diagnostic.ImpactAbort),
		ExecFn:  harness.PanicExecFn("A.Execute must not be called"),
	}
	opB := &harness.FakeOp{
		Name:    "B",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("B.Execute must not be called"),
	}

	opB.Deps = []spec.Op{opA}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA, opB),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(ctx, plan)
	if err == nil {
		t.Fatalf("expected abort error")
	}

	ar := mustSingleAction(t, rep)
	mustOpOutcome(t, ar, "A", model.OpAborted)
	mustOpOutcome(t, ar, "B", model.OpAborted)
}

// Dependency graph:
//
//	A → B → C
//
// Behavior:
//
//	A.Execute → Success
//	B.Execute → Abort
//	C.Execute → MUST NOT be called
func TestExecuteAction_AbortDuringExecution(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.OkExecFn(true),
	}
	opB := &harness.FakeOp{
		Name:    "B",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.DiagExecFn(signal.Error, diagnostic.ImpactAbort),
	}
	opC := &harness.FakeOp{
		Name:    "C",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("C.Execute must not be called"),
	}

	opB.Deps = []spec.Op{opA}
	opC.Deps = []spec.Op{opB}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA, opB, opC),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(ctx, plan)
	if err == nil {
		t.Fatalf("expected abort error")
	}

	ar := mustSingleAction(t, rep)
	mustOpOutcome(t, ar, "A", model.OpSucceeded)
	mustOpOutcome(t, ar, "B", model.OpFailed)
	mustOpOutcome(t, ar, "C", model.OpAborted)
}

// Dependency graph:
//
//	A → B
//
// Behavior:
//
//	A.Check   → Satisfied (Skipped)
//	A.Execute → MUST NOT be called
//	B.Execute → Success
func TestExecuteAction_SkippedUpstream_ExecutesDownstream(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.PanicExecFn("A.Execute must not be called"),
	}

	opB := &harness.FakeOp{
		Name:    "B",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.OkExecFn(true),
	}

	// wire dependencies
	opB.Deps = []spec.Op{opA}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA, opB),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(ctx, plan)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	if opA.CheckCalls != 1 || opB.CheckCalls != 1 {
		t.Fatalf(
			"expected Check() to be called once for all ops, got A=%d B=%d",
			opA.CheckCalls,
			opB.CheckCalls,
		)
	}

	if opA.ExecCalls != 0 {
		t.Fatalf("expected A not to execute, got %d", opA.ExecCalls)
	}

	if opB.ExecCalls != 1 {
		t.Fatalf("expected B to execute once, got %d", opB.ExecCalls)
	}

	ar := mustSingleAction(t, rep)
	mustOpOutcome(t, ar, "A", model.OpSkipped)
	mustOpOutcome(t, ar, "B", model.OpSucceeded)
}

func mustSingleAction(t *testing.T, rep model.ExecutionReport) model.ActionReport {
	t.Helper()

	if len(rep.Actions) != 1 {
		t.Fatalf("expected exactly 1 action, got %d", len(rep.Actions))
	}
	return rep.Actions[0]
}

func mustOpOutcome(
	t *testing.T,
	ar model.ActionReport,
	opName string,
	want model.OpOutcome,
) model.OpReport {
	t.Helper()

	for _, op := range ar.Ops {
		fo, ok := op.Op.(*harness.FakeOp)
		if ok && fo.Name == opName {
			if op.Outcome != want {
				t.Fatalf(
					"op %q: expected outcome %v, got %v",
					opName,
					want,
					op.Outcome,
				)
			}
			return op
		}
	}

	t.Fatalf("op %q not found in action report", opName)
	return model.OpReport{}
}
