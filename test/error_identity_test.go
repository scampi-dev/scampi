// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"errors"
	"testing"

	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/local"
)

func TestCheck_RawErrorInOpCheck_PropagatesAndPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for raw check error")
		}
	}()

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	op := &fakeOp{
		name: "raw-error-op",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, errors.New("random check error")
		},
		execFn: panicExecFn("exec must not run on raw check error"),
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

	_, err = e.ExecutePlan(ctx, plan)

	// panicIfNotAbortError should trigger
	_ = err
}

func TestCheck_RawErrorInOpExec_PropagatesAndPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for raw exec error")
		}
	}()

	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	op := &fakeOp{
		name: "raw-error-op",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		execFn: func(context.Context, source.Source, target.Target) (spec.Result, error) {
			return spec.Result{}, errors.New("random exec error")
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

	_, err = e.ExecutePlan(ctx, plan)

	// panicIfNotAbortError should trigger
	_ = err
}

// Cancellation
// -----------------------------------------------------------------------------

func TestExecutePlan_CancelledContext_ReturnsCancelledError(t *testing.T) {
	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := noopEmitter{}

	ctx, cancel := context.WithCancel(context.Background())
	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(tgt),
	}

	e, err := engine.New(ctx, src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	op := &fakeOp{
		name: "slow-op",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, nil
		},
		execFn: func(ctx context.Context, _ source.Source, _ target.Target) (spec.Result, error) {
			cancel()
			return spec.Result{}, ctx.Err()
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

	_, err = e.ExecutePlan(ctx, plan)

	var cancelled engine.CancelledError
	if !errors.As(err, &cancelled) {
		t.Fatalf("expected CancelledError, got %T: %v", err, err)
	}
}
