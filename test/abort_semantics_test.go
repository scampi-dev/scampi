package test

import (
	"context"
	"sync/atomic"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

// ASSUMPTION:
// - spec.Op.Check may return an error implementing DiagnosticsProvider
// - diagnostics policy allows non-aborting diagnostics
// - execution must continue pessimistically
func TestCheck_NonAbortingDiagnostics_DoNotAbort(t *testing.T) {
	ctx := context.Background()

	recTgt := &target.Recorder{Inner: target.LocalPosixTarget{}}
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	var execRan atomic.Bool

	op := &fakeOp{
		name: "A",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
			return spec.CheckUnsatisfied, fakeDiagnostic{
				severity: signal.Warning,
				impact:   diagnostic.ImpactNone,
			}
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			execRan.Store(true)
			return spec.Result{Changed: true}, nil
		},
	}

	act := &fakeAction{
		ops: []spec.Op{op},
	}
	op.action = act

	plan := spec.Plan{
		Actions: []spec.Action{act},
	}

	e := engine.New(source.LocalPosixSource{}, recTgt, em)
	res, err := e.ExecutePlan(ctx, plan)
	if err != nil {
		t.Fatalf("non-aborting diagnostics must not return error, got %v", err)
	}

	if !execRan.Load() {
		t.Fatalf("op with warning diagnostics must be treated as needing execution")
	}

	if !res[0].Res.Changed {
		t.Fatalf("op with warning diagnostics must execute")
	}
}

func TestCheck_NonAbortDiagnostic_AllowsSiblingOps(t *testing.T) {
	ctx := context.Background()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	var ranA, ranB atomic.Bool

	opA := &fakeOp{
		name: "A",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
			return spec.CheckUnsatisfied, fakeDiagnostic{
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
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
			return spec.CheckUnsatisfied, nil
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranB.Store(true)
			return spec.Result{Changed: true}, nil
		},
	}

	act := &fakeAction{
		ops: []spec.Op{opA, opB},
	}
	opA.action = act
	opB.action = act

	plan := spec.Plan{
		Actions: []spec.Action{act},
	}

	e := engine.New(source.LocalPosixSource{}, target.LocalPosixTarget{}, em)
	_, err := e.ExecutePlan(ctx, plan)
	if err != nil {
		t.Fatalf("non-abort diagnostics must not fail execution: %v", err)
	}

	if !ranA.Load() || !ranB.Load() {
		t.Fatalf("all sibling ops must execute despite non-abort diagnostics")
	}
}

func TestCheck_AbortDiagnostic_StopsSiblingOps(t *testing.T) {
	ctx := context.Background()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	opA := &fakeOp{
		name: "A",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
			return spec.CheckUnsatisfied, fakeDiagnostic{
				severity: signal.Error,
				impact:   diagnostic.ImpactAbort,
			}
		},
		execFn: panicExecFn("execFn of A must not be called after aborting check"),
	}

	opB := &fakeOp{
		name: "B",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
			return spec.CheckUnsatisfied, nil
		},
		execFn: panicExecFn("execFn of B must not be called after aborting check"),
	}

	act := &fakeAction{
		ops: []spec.Op{opA, opB},
	}
	opA.action = act
	opB.action = act

	plan := spec.Plan{
		Actions: []spec.Action{act},
	}

	e := engine.New(source.LocalPosixSource{}, target.LocalPosixTarget{}, em)
	_, err := e.ExecutePlan(ctx, plan)

	if err == nil {
		t.Fatalf("abort diagnostic must abort execution")
	}
}
