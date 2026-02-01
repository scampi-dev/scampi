package test

import (
	"context"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/model"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target/local"
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
	opA := &fakeOp{
		name:    "A",
		checkFn: okCheckFn(spec.CheckSatisfied),
		execFn:  panicExecFn("A.Execute must not be called"),
	}
	opB := &fakeOp{
		name:    "B",
		checkFn: okCheckFn(spec.CheckSatisfied),
		execFn:  panicExecFn("B.Execute must not be called"),
	}
	opC := &fakeOp{
		name:    "C",
		checkFn: okCheckFn(spec.CheckSatisfied),
		execFn:  panicExecFn("C.Execute must not be called"),
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA, opB, opC),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
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

	if opA.checkCalls != 1 || opB.checkCalls != 1 || opC.checkCalls != 1 {
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
	opA := &fakeOp{
		name:    "A",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  okExecFn(true),
	}
	opB := &fakeOp{
		name:    "B",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  okExecFn(false),
	}
	opC := &fakeOp{
		name:    "C",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  okExecFn(true),
	}

	opB.deps = []spec.Op{opA}
	opC.deps = []spec.Op{opB}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA, opB, opC),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
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

	if opA.execCalls != 1 || opB.execCalls != 1 || opC.execCalls != 1 {
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
	var act *fakeAction

	opA := &fakeOp{
		name:    "A",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  okExecFn(true),
	}

	opB := &fakeOp{
		name:    "B",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  diagExecFn(signal.Error, diagnostic.ImpactAbort),
	}

	opC := &fakeOp{
		name:    "C",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  panicExecFn("C.Execute must not be called"),
	}

	opB.deps = []spec.Op{opA}
	opC.deps = []spec.Op{opB}

	act = mkAction(opA, opB, opC)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				act,
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
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

	if opA.checkCalls != 1 || opB.checkCalls != 1 || opC.checkCalls != 1 {
		t.Fatalf("expected Check() to be called once for all ops, got A=%d B=%d C=%d",
			opA.checkCalls, opB.checkCalls, opC.checkCalls)
	}

	if opA.execCalls != 1 {
		t.Fatalf("expected A to execute once, got %d", opA.execCalls)
	}
	if opB.execCalls != 1 {
		t.Fatalf("expected B to execute once, got %d", opB.execCalls)
	}
	if opC.execCalls != 0 {
		t.Fatalf("expected C not to execute, got %d", opC.execCalls)
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
	opA := &fakeOp{
		name:    "A",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  okExecFn(true),
	}
	opB := &fakeOp{
		name:    "B",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  okExecFn(true),
	}
	opC := &fakeOp{
		name:    "C",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  diagExecFn(signal.Error, diagnostic.ImpactAbort),
	}
	opD := &fakeOp{
		name:    "D",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  panicExecFn("D.Execute must not be called"),
	}

	opB.deps = []spec.Op{opA}
	opC.deps = []spec.Op{opA}
	opD.deps = []spec.Op{opC}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA, opB, opC, opD),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
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

	if opB.execCalls != 1 {
		t.Fatalf("expected B to execute")
	}
	if opD.execCalls != 0 {
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
	opA := &fakeOp{
		name:    "A",
		checkFn: diagCheckFn(signal.Warning, diagnostic.ImpactNone),
		execFn:  okExecFn(true),
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
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

	if opA.execCalls != 1 {
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
	opA := &fakeOp{
		name:    "A",
		checkFn: diagCheckFn(signal.Error, diagnostic.ImpactAbort),
		execFn:  panicExecFn("A.Execute must not be called"),
	}
	opB := &fakeOp{
		name:    "B",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  panicExecFn("B.Execute must not be called"),
	}

	opB.deps = []spec.Op{opA}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA, opB),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
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
	opA := &fakeOp{
		name:    "A",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  okExecFn(true),
	}
	opB := &fakeOp{
		name:    "B",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  diagExecFn(signal.Error, diagnostic.ImpactAbort),
	}
	opC := &fakeOp{
		name:    "C",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  panicExecFn("C.Execute must not be called"),
	}

	opB.deps = []spec.Op{opA}
	opC.deps = []spec.Op{opB}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA, opB, opC),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
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
	opA := &fakeOp{
		name:    "A",
		checkFn: okCheckFn(spec.CheckSatisfied),
		execFn:  panicExecFn("A.Execute must not be called"),
	}

	opB := &fakeOp{
		name:    "B",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  okExecFn(true),
	}

	// wire dependencies
	opB.deps = []spec.Op{opA}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA, opB),
			},
		},
	}

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
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

	if opA.checkCalls != 1 || opB.checkCalls != 1 {
		t.Fatalf(
			"expected Check() to be called once for all ops, got A=%d B=%d",
			opA.checkCalls,
			opB.checkCalls,
		)
	}

	if opA.execCalls != 0 {
		t.Fatalf("expected A not to execute, got %d", opA.execCalls)
	}

	if opB.execCalls != 1 {
		t.Fatalf("expected B to execute once, got %d", opB.execCalls)
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
		fo, ok := op.Op.(*fakeOp)
		if ok && fo.name == opName {
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
