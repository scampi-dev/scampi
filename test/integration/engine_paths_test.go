// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/local"
	"scampi.dev/scampi/test/harness"
)

// Check-side twin of Test_ExecutePlan_CancelledContextReturnsCancelledError:
// a context cancelled mid-check surfaces as CancelledError, not a raw
// context.Canceled or a BUG panic.
func Test_CheckPlan_CancelledContextReturnsCancelledError(t *testing.T) {
	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter()

	ctx, cancel := context.WithCancel(t.Context())
	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(tgt),
	}

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, cfg)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	op := &harness.FakeOp{
		Name: "cancelling-check",
		CheckFn: func(
			ctx context.Context,
			_ source.Source,
			_ target.Target,
		) (spec.CheckResult, []spec.DriftDetail, error) {
			cancel()
			return spec.CheckUnknown, nil, ctx.Err()
		},
		ExecFn: harness.PanicExecFn("exec must not run during check"),
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

	_, _, err = e.CheckPlan(diagnostic.NewCtx(ctx, em), plan)

	var cancelled engine.CancelledError
	if !errors.As(err, &cancelled) {
		t.Fatalf("expected CancelledError, got %T: %v", err, err)
	}
}

// Per-op timeout
// -----------------------------------------------------------------------------

// timedOp is a FakeOp that declares its own timeout via spec.OpTimeout.
type timedOp struct {
	*harness.FakeOp
	timeout time.Duration
}

func (o *timedOp) Timeout() time.Duration { return o.timeout }

// Every op runs under a deadline-bound context: ops implementing
// spec.OpTimeout get their declared timeout, everything else the engine
// default (30s). A hung op that honors its context can therefore not stall a
// run forever.
func Test_CheckPlan_OpTimeoutBoundsOpContexts(t *testing.T) {
	src := source.LocalPosixSource{}
	tgt := local.POSIXTarget{}
	em := harness.NoopEmitter()
	ctx := t.Context()

	var customRemaining, defaultRemaining atomic.Int64
	record := func(slot *atomic.Int64) harness.CheckFn {
		return func(
			ctx context.Context,
			_ source.Source,
			_ target.Target,
		) (spec.CheckResult, []spec.DriftDetail, error) {
			d, ok := ctx.Deadline()
			if !ok {
				t.Error("op context has no deadline")
				return spec.CheckSatisfied, nil, nil
			}
			slot.Store(int64(time.Until(d)))
			return spec.CheckSatisfied, nil, nil
		}
	}

	custom := &timedOp{
		FakeOp:  &harness.FakeOp{Name: "custom-timeout", CheckFn: record(&customRemaining)},
		timeout: 5 * time.Minute,
	}
	def := &harness.FakeOp{Name: "default-timeout", CheckFn: record(&defaultRemaining)}

	step := &harness.FakeStep{}
	step.AddOp(custom)
	step.AddOp(def)
	custom.SetStep(step)
	def.SetStep(step)

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(tgt),
	}
	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, cfg)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:         "fakeUnit",
			TargetName: "fakeUnit description",
			Steps:      []spec.Step{step},
		},
	}
	if _, _, err := e.CheckPlan(diagnostic.NewCtx(ctx, em), plan); err != nil {
		t.Fatalf("CheckPlan: %v", err)
	}

	if got := time.Duration(customRemaining.Load()); got <= 30*time.Second || got > 5*time.Minute {
		t.Errorf("custom op deadline remaining = %v, want ~5m (spec.OpTimeout not honored)", got)
	}
	if got := time.Duration(defaultRemaining.Load()); got <= 0 || got > 30*time.Second {
		t.Errorf("default op deadline remaining = %v, want within the 30s default", got)
	}
}
