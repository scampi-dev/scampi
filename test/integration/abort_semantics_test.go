// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/local"
	"scampi.dev/scampi/test/harness"
)

// ASSUMPTION:
// - spec.Op.Check may return an error implementing DiagnosticsProvider
// - diagnostics policy allows non-aborting diagnostics
// - execution must continue pessimistically
func TestCheck_NonAbortingDiagnostics_DoNotAbort(t *testing.T) {
	var execRan atomic.Bool

	op := &harness.FakeOp{
		Name: "A",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, harness.NewFakeDiagnostic(signal.Warning, diagnostic.ImpactNone, nil)
		},
		ExecFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			execRan.Store(true)
			return spec.Result{Changed: true}, nil
		},
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(op),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(
		context.Background(),

		harness.NoopEmitter{}),

		source.LocalPosixSource{},
		cfg)

	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	rep, err := e.ExecutePlan(diagnostic.NewCtx(context.Background(), harness.NoopEmitter{}), plan)
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

	if opRep.Outcome != result.OpSucceeded {
		t.Fatalf("expected op to succeed, got outcome %v", opRep.Outcome)
	}

	if opRep.Result == nil || !opRep.Result.Changed {
		t.Fatalf("op with warning diagnostics must execute and report Changed")
	}
}

func TestCheck_NonAbortDiagnostic_AllowsSiblingOps(t *testing.T) {
	var ranA, ranB atomic.Bool

	opA := &harness.FakeOp{
		Name: "A",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, harness.NewFakeDiagnostic(signal.Warning, diagnostic.ImpactNone, nil)
		},
		ExecFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranA.Store(true)
			return spec.Result{Changed: true}, nil
		},
	}

	opB := &harness.FakeOp{
		Name: "B",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		ExecFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranB.Store(true)
			return spec.Result{Changed: true}, nil
		},
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID: "fakeUnit",
			Actions: []spec.Action{
				harness.MkAction(opA, opB),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(
		context.Background(),

		harness.NoopEmitter{}),

		source.LocalPosixSource{},
		cfg)

	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(diagnostic.NewCtx(context.Background(), harness.NoopEmitter{}), plan)
	if err != nil {
		t.Fatalf("non-abort diagnostics must not fail execution: %v", err)
	}

	if !ranA.Load() || !ranB.Load() {
		t.Fatalf("all sibling ops must execute despite non-abort diagnostics")
	}
}

func TestCheck_AbortDiagnostic_StopsSiblingOps(t *testing.T) {
	opA := &harness.FakeOp{
		Name: "A",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, harness.NewFakeDiagnostic(signal.Error, diagnostic.ImpactAbort, nil)
		},
		ExecFn: harness.PanicExecFn("harness.ExecFn of A must not be called after aborting check"),
	}

	opB := &harness.FakeOp{
		Name: "B",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		ExecFn: harness.PanicExecFn("harness.ExecFn of B must not be called after aborting check"),
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA, opB),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(
		context.Background(),

		harness.NoopEmitter{}),

		source.LocalPosixSource{},
		cfg)

	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(diagnostic.NewCtx(context.Background(), harness.NoopEmitter{}), plan)

	if err == nil {
		t.Fatalf("abort diagnostic must abort execution")
	}
}

func TestCheck_AbortDiagnostic_StopsActionExecution(t *testing.T) {
	op := &harness.FakeOp{
		Name: "abort-op",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, harness.NewFakeDiagnostic(signal.Error, diagnostic.ImpactAbort, nil)
		},
		ExecFn: harness.PanicExecFn("exec must not run after aborting check"),
	}
	noExecOp := &harness.FakeOp{
		Name:    "no-exec-op",
		CheckFn: harness.PanicCheckFn("check must not run after abort"),
		ExecFn:  harness.PanicExecFn("exec must not run after aborting check"),
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(op),
				harness.MkAction(noExecOp),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(
		context.Background(),

		harness.NoopEmitter{}),

		source.LocalPosixSource{},
		cfg)

	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(diagnostic.NewCtx(context.Background(), harness.NoopEmitter{}), plan)

	if err == nil {
		t.Fatalf("abort diagnostic must abort execution")
	}
}

func TestCheck_NonAbortDiagnostic_AllowsSiblingExecution(t *testing.T) {
	var ranA, ranB bool

	opA := &harness.FakeOp{
		Name: "A",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, harness.NewFakeDiagnostic(signal.Warning, diagnostic.ImpactNone, nil)
		},
		ExecFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranA = true
			return spec.Result{Changed: true}, nil
		},
	}

	opB := &harness.FakeOp{
		Name: "B",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		ExecFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			ranB = true
			return spec.Result{Changed: true}, nil
		},
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(opA, opB),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(
		context.Background(),

		harness.NoopEmitter{}),

		source.LocalPosixSource{},
		cfg)

	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(diagnostic.NewCtx(context.Background(), harness.NoopEmitter{}), plan)
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
	parent := &harness.FakeOp{
		Name: "parent",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		ExecFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			return spec.Result{}, harness.NewFakeDiagnostic(signal.Error, diagnostic.ImpactAbort, nil)
		},
	}

	// child op: must never execute
	child := &harness.FakeOp{
		Name: "child",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		ExecFn: harness.PanicExecFn("exec must not run after parent failure"),
	}

	// dependency: child depends on parent
	child.Deps = append(child.Deps, parent)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:   "fakeUnit",
			Desc: "fakeUnit description",
			Actions: []spec.Action{
				harness.MkAction(parent, child),
			},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(
		context.Background(),

		harness.NoopEmitter{}),

		source.LocalPosixSource{},
		cfg)

	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(diagnostic.NewCtx(ctx, harness.NoopEmitter{}), plan)

	if err == nil {
		t.Fatalf("execution error must propagate")
	}
}
