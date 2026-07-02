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

// Ref wire path (ResolveRefs before a step runs, captureStepOutput after)
// -----------------------------------------------------------------------------

// outputOp is a FakeOp whose settled output feeds the engine's ref registry.
type outputOp struct {
	*harness.FakeOp
	out any
}

func (o *outputOp) Output() any { return o.out }

// refProducerStep is a FakeStep with a StepID, so the engine captures its
// op outputs.
type refProducerStep struct {
	*harness.FakeStep
	id spec.StepID
}

func (s *refProducerStep) StepID() spec.StepID { return s.id }

// refConsumerStep records the value the engine resolved for its ref.
type refConsumerStep struct {
	*harness.FakeStep
	ref      spec.Ref
	resolved atomic.Value
}

func (s *refConsumerStep) ResolveRefs(r spec.RefResolver) error {
	v, err := r(s.ref)
	if err != nil {
		return err
	}
	s.resolved.Store(v)
	return nil
}

func refPlanFixture(id spec.StepID, out any, expr string) (*refProducerStep, *refConsumerStep, spec.Plan) {
	prodOp := &outputOp{
		FakeOp: &harness.FakeOp{
			Name:    "produce",
			CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
			ExecFn:  harness.OkExecFn(true),
		},
		out: out,
	}
	producer := &refProducerStep{FakeStep: &harness.FakeStep{}, id: id}
	producer.AddOp(prodOp)
	prodOp.SetStep(producer)

	consOp := &harness.FakeOp{
		Name:    "consume",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.OkExecFn(false),
	}
	consumer := &refConsumerStep{
		FakeStep: &harness.FakeStep{},
		ref:      spec.Ref{TargetID: id, Expr: expr},
	}
	consumer.AddOp(consOp)
	consOp.SetStep(consumer)

	// Neither step declares resources, so both are barriers and run
	// sequentially in declaration order: producer settles first.
	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:         "fakeUnit",
			TargetName: "fakeUnit description",
			Steps:      []spec.Step{producer, consumer},
		},
	}
	return producer, consumer, plan
}

func newRefEngine(t *testing.T) *engine.Engine {
	t.Helper()
	em := harness.NoopEmitter()
	cfg := spec.Config{Target: harness.MockDeclaredTarget(local.POSIXTarget{})}
	e, err := engine.New(diagnostic.NewCtx(t.Context(), em), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	t.Cleanup(e.Close)
	return e
}

// CheckPlan resolves a downstream ref from the output an upstream step
// settled during the same check pass.
func Test_CheckPlan_ResolvesRefFromUpstreamOutput(t *testing.T) {
	out := map[string]any{"instance": map[string]any{"id": "srv-42"}}
	_, consumer, plan := refPlanFixture(7, out, ".instance.id")

	e := newRefEngine(t)
	em := harness.NoopEmitter()
	if _, _, err := e.CheckPlan(diagnostic.NewCtx(t.Context(), em), plan); err != nil {
		t.Fatalf("CheckPlan: %v", err)
	}

	if got := consumer.resolved.Load(); got != "srv-42" {
		t.Errorf("resolved ref = %v (%T), want %q", got, got, "srv-42")
	}
}

// ExecutePlan wires the same path on the execute side.
func Test_ExecutePlan_ResolvesRefFromUpstreamOutput(t *testing.T) {
	out := map[string]any{"instance": map[string]any{"id": "srv-42"}}
	_, consumer, plan := refPlanFixture(7, out, ".instance.id")

	e := newRefEngine(t)
	em := harness.NoopEmitter()
	if _, err := e.ExecutePlan(diagnostic.NewCtx(t.Context(), em), plan); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	if got := consumer.resolved.Load(); got != "srv-42" {
		t.Errorf("resolved ref = %v (%T), want %q", got, got, "srv-42")
	}
}

// In execute mode a ref to a step that never produced output is an abort,
// not a silent nil.
func Test_ExecutePlan_AbortsOnRefWithoutOutput(t *testing.T) {
	_, consumer, plan := refPlanFixture(7, nil, ".instance.id")
	consumer.ref.TargetID = 99 // nobody produces this ID

	e := newRefEngine(t)
	em := harness.NoopEmitter()
	_, err := e.ExecutePlan(diagnostic.NewCtx(t.Context(), em), plan)

	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}
	if consumer.resolved.Load() != nil {
		t.Errorf("consumer resolved a value from a missing output: %v", consumer.resolved.Load())
	}
}
