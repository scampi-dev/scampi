package test

import (
	"context"
	"sync/atomic"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/model"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"godoit.dev/doit/target/local"
)

// ASSUMPTION:
// - spec.Op.Check may return an error implementing DiagnosticsProvider
// - diagnostics policy allows non-aborting diagnostics
// - execution must continue pessimistically
func TestCheck_NonAbortingDiagnostics_DoNotAbort(t *testing.T) {
	var execRan atomic.Bool

	op := &fakeOp{
		name: "A",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, fakeDiagnostic{
				severity: signal.Warning,
				impact:   diagnostic.ImpactNone,
			}
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			execRan.Store(true)
			return spec.Result{Changed: true}, nil
		},
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(op),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(
		context.Background(),
		source.LocalPosixSource{},
		cfg,
		noopEmitter{},
	)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("non-aborting diagnostics must not return error, got %v", err)
	}

	if !execRan.Load() {
		t.Fatalf("op with warning diagnostics must be treated as needing execution")
	}

	if len(rep.Actions) != 1 {
		t.Fatalf("expected exactly one action report")
	}

	ar := rep.Actions[0]

	if len(ar.Ops) != 1 {
		t.Fatalf("expected exactly one op report")
	}

	opRep := ar.Ops[0]

	if opRep.Outcome != model.OpSucceeded {
		t.Fatalf("expected op to succeed, got outcome %v", opRep.Outcome)
	}

	if opRep.Result == nil || !opRep.Result.Changed {
		t.Fatalf("op with warning diagnostics must execute and report Changed")
	}
}

func TestCheck_NonAbortDiagnostic_AllowsSiblingOps(t *testing.T) {
	var ranA, ranB atomic.Bool

	opA := &fakeOp{
		name: "A",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, fakeDiagnostic{
				severity: signal.Warning,
				impact:   diagnostic.ImpactNone,
			}
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranA.Store(true)
			return spec.Result{Changed: true}, nil
		},
	}

	opB := &fakeOp{
		name: "B",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranB.Store(true)
			return spec.Result{Changed: true}, nil
		},
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID: "fakeUnit",
			Actions: []spec.Action{
				mkAction(opA, opB),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(
		context.Background(),
		source.LocalPosixSource{},
		cfg,
		noopEmitter{},
	)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("non-abort diagnostics must not fail execution: %v", err)
	}

	if !ranA.Load() || !ranB.Load() {
		t.Fatalf("all sibling ops must execute despite non-abort diagnostics")
	}
}

func TestCheck_AbortDiagnostic_StopsSiblingOps(t *testing.T) {
	opA := &fakeOp{
		name: "A",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, fakeDiagnostic{
				severity: signal.Error,
				impact:   diagnostic.ImpactAbort,
			}
		},
		execFn: panicExecFn("execFn of A must not be called after aborting check"),
	}

	opB := &fakeOp{
		name: "B",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		execFn: panicExecFn("execFn of B must not be called after aborting check"),
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA, opB),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(
		context.Background(),
		source.LocalPosixSource{},
		cfg,
		noopEmitter{},
	)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(context.Background(), plan)

	if err == nil {
		t.Fatalf("abort diagnostic must abort execution")
	}
}

func TestCheck_AbortDiagnostic_StopsActionExecution(t *testing.T) {
	op := &fakeOp{
		name: "abort-op",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, fakeDiagnostic{
				severity: signal.Error,
				impact:   diagnostic.ImpactAbort,
			}
		},
		execFn: panicExecFn("exec must not run after aborting check"),
	}
	noExecOp := &fakeOp{
		name:    "no-exec-op",
		checkFn: panicCheckFn("check must not run after abort"),
		execFn:  panicExecFn("exec must not run after aborting check"),
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(op),
				mkAction(noExecOp),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(
		context.Background(),
		source.LocalPosixSource{},
		cfg,
		noopEmitter{},
	)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(context.Background(), plan)

	if err == nil {
		t.Fatalf("abort diagnostic must abort execution")
	}
}

func TestCheck_NonAbortDiagnostic_AllowsSiblingExecution(t *testing.T) {
	var ranA, ranB bool

	opA := &fakeOp{
		name: "A",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, fakeDiagnostic{
				severity: signal.Warning,
				impact:   diagnostic.ImpactNone,
			}
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranA = true
			return spec.Result{Changed: true}, nil
		},
	}

	opB := &fakeOp{
		name: "B",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranB = true
			return spec.Result{Changed: true}, nil
		},
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(opA, opB),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(
		context.Background(),
		source.LocalPosixSource{},
		cfg,
		noopEmitter{},
	)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("non-abort diagnostics must not fail execution: %v", err)
	}

	if !ranA || !ranB {
		t.Fatalf("all sibling ops must execute")
	}
}

func TestExecute_FailedOp_BlocksDependentOps(t *testing.T) {
	ctx := context.Background()

	// parent op: executes and fails
	parent := &fakeOp{
		name: "parent",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			return spec.Result{}, fakeDiagnostic{
				severity: signal.Error,
				impact:   diagnostic.ImpactAbort,
			}
		},
	}

	// child op: must never execute
	child := &fakeOp{
		name: "child",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		execFn: panicExecFn("exec must not run after parent failure"),
	}

	// dependency: child depends on parent
	child.deps = append(child.deps, parent)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				mkAction(parent, child),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(
		context.Background(),
		source.LocalPosixSource{},
		cfg,
		noopEmitter{},
	)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(ctx, plan)

	if err == nil {
		t.Fatalf("execution error must propagate")
	}
}
