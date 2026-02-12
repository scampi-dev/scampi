package test

import (
	"context"
	"testing"

	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"godoit.dev/doit/target/local"
)

func driftCheckFn(drift []spec.DriftDetail) checkFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		return spec.CheckUnsatisfied, drift, nil
	}
}

// CheckPlan on an unsatisfied op emits drift detail in the OpChecked event.
func TestCheckPlan_DriftEmitted(t *testing.T) {
	want := []spec.DriftDetail{
		{Field: "content", Current: "100 bytes", Desired: "200 bytes"},
	}

	opA := &fakeOp{
		name:    "A",
		checkFn: driftCheckFn(want),
		execFn:  panicExecFn("must not execute in check mode"),
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "driftUnit",
			Desc:    "drift test",
			Actions: []spec.Action{mkAction(opA)},
		},
	}

	rec := &recordingDisplayer{}
	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(ctx, source.LocalPosixSource{}, cfg, rec)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, err = e.CheckPlan(ctx, plan)
	if err != nil {
		t.Fatalf("CheckPlan: %v", err)
	}

	var found bool
	for _, ev := range rec.opEvents {
		if ev.Kind != event.OpChecked || ev.CheckDetail == nil {
			continue
		}
		if ev.CheckDetail.Result != spec.CheckUnsatisfied {
			continue
		}
		if len(ev.CheckDetail.Drift) == 0 {
			t.Fatal("expected drift detail on unsatisfied op, got nil")
		}
		d := ev.CheckDetail.Drift[0]
		if d.Field != want[0].Field ||
			d.Current != want[0].Current ||
			d.Desired != want[0].Desired {
			t.Fatalf(
				"drift mismatch:\n  got  %+v\n  want %+v",
				d, want[0],
			)
		}
		found = true
	}
	if !found {
		t.Fatal("no OpChecked event with CheckUnsatisfied found")
	}
}

// CheckPlan on a satisfied op emits no drift detail.
func TestCheckPlan_SatisfiedNoDrift(t *testing.T) {
	opA := &fakeOp{
		name:    "A",
		checkFn: okCheckFn(spec.CheckSatisfied),
		execFn:  panicExecFn("must not execute"),
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "driftUnit",
			Desc:    "drift test",
			Actions: []spec.Action{mkAction(opA)},
		},
	}

	rec := &recordingDisplayer{}
	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(ctx, source.LocalPosixSource{}, cfg, rec)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, err = e.CheckPlan(ctx, plan)
	if err != nil {
		t.Fatalf("CheckPlan: %v", err)
	}

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpChecked && ev.CheckDetail != nil {
			if len(ev.CheckDetail.Drift) > 0 {
				t.Fatal("satisfied op should have no drift")
			}
		}
	}
}

// ExecutePlan does NOT emit drift (even on unsatisfied ops).
func TestExecutePlan_NoDrift(t *testing.T) {
	opA := &fakeOp{
		name:    "A",
		checkFn: driftCheckFn([]spec.DriftDetail{{Field: "content", Current: "x", Desired: "y"}}),
		execFn:  okExecFn(true),
	}

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "driftUnit",
			Desc:    "drift test",
			Actions: []spec.Action{mkAction(opA)},
		},
	}

	rec := &recordingDisplayer{}
	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(ctx, source.LocalPosixSource{}, cfg, rec)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(ctx, plan)
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpChecked && ev.CheckDetail != nil {
			if len(ev.CheckDetail.Drift) > 0 {
				t.Fatal("apply mode should not emit drift")
			}
		}
	}
}
