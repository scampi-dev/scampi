// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/controller"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/local"
	"scampi.dev/scampi/test/harness"
)

// assertEnginePanic verifies the engine raised a BUG panic wrapping origMsg.
// Bare `recover() == nil` checks let unrelated panics (nil derefs, etc.) pass
// the test silently - this asserts on the panic value's shape and content.
// Caller invokes via `defer func() { assertEnginePanic(t, recover(), msg) }()`.
func assertEnginePanic(t *testing.T, r any, origMsg string) {
	t.Helper()
	if r == nil {
		t.Fatalf("expected panic, got none")
	}
	err, ok := r.(error)
	if !ok {
		t.Fatalf("expected panic value to be error, got %T: %v", r, r)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "BUG:") {
		t.Fatalf("expected BUG panic, got: %v", err)
	}
	if !strings.Contains(msg, origMsg) {
		t.Fatalf("expected panic to wrap %q, got: %v", origMsg, err)
	}
}

func Test_Check_RawErrorInOpCheckPropagatesAndPanics(t *testing.T) {
	defer func() { assertEnginePanic(t, recover(), "random check error") }()

	ctl := controller.Posix{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter()

	ctx := t.Context()
	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(tgt),
	}

	e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, cfg)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	op := &harness.FakeOp{
		Name: "raw-error-op",
		CheckFn: func(
			context.Context,
			controller.Controller,
			target.Target,
		) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, errors.New("random check error")
		},
		ExecFn: harness.PanicExecFn("exec must not run on raw check error"),
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:         "fakeUnit",
			TargetName: "fakeUnit description",
			Steps: []spec.Step{
				harness.MkStep(op),
			},
		},
	}

	_, err = e.ExecutePlan(diagnostic.NewCtx(ctx, em), plan)

	// panicIfNotAbortError should trigger
	_ = err
}

func Test_Check_RawErrorInOpExecPropagatesAndPanics(t *testing.T) {
	defer func() { assertEnginePanic(t, recover(), "random exec error") }()

	ctl := controller.Posix{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter()

	ctx := t.Context()
	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(tgt),
	}

	e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, cfg)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	op := &harness.FakeOp{
		Name: "raw-error-op",
		CheckFn: func(
			context.Context,
			controller.Controller,
			target.Target,
		) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		ExecFn: func(context.Context, controller.Controller, target.Target) (spec.Result, error) {
			return spec.Result{}, errors.New("random exec error")
		},
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:         "fakeUnit",
			TargetName: "fakeUnit description",
			Steps: []spec.Step{
				harness.MkStep(op),
			},
		},
	}

	_, err = e.ExecutePlan(diagnostic.NewCtx(ctx, em), plan)

	// panicIfNotAbortError should trigger
	_ = err
}

// Cancellation
// -----------------------------------------------------------------------------

func Test_ExecutePlan_CancelledContextReturnsCancelledError(t *testing.T) {
	ctl := controller.Posix{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter()

	ctx, cancel := context.WithCancel(t.Context())
	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(tgt),
	}

	e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, cfg)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	op := &harness.FakeOp{
		Name: "slow-op",
		CheckFn: func(
			context.Context,
			controller.Controller,
			target.Target,
		) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		ExecFn: func(ctx context.Context, _ controller.Controller, _ target.Target) (spec.Result, error) {
			cancel()
			return spec.Result{}, ctx.Err()
		},
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:         "fakeUnit",
			TargetName: "fakeUnit description",
			Steps: []spec.Step{
				harness.MkStep(op),
			},
		},
	}

	_, err = e.ExecutePlan(diagnostic.NewCtx(ctx, em), plan)

	var cancelled engine.CancelledError
	if !errors.As(err, &cancelled) {
		t.Fatalf("expected CancelledError, got %T: %v", err, err)
	}
}
