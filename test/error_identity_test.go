package test

import (
	"context"
	"errors"
	"testing"

	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"godoit.dev/doit/target/local"
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
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
	}

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	op := &fakeOp{
		name: "raw-error-op",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
			return spec.CheckUnsatisfied, errors.New("random check error")
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
	cfg := spec.Config{
		Target: mockTargetInstance(tgt),
	}

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	op := &fakeOp{
		name: "raw-error-op",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
			return spec.CheckUnsatisfied, nil
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
