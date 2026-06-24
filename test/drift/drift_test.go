// SPDX-License-Identifier: GPL-3.0-only

package drift

import (
	"context"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/local"
	"scampi.dev/scampi/test/harness"
)

func driftCheckFn(drift []spec.DriftDetail) harness.CheckFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		return spec.CheckUnsatisfied, drift, nil
	}
}

// CheckPlan on an unsatisfied op emits drift detail in the OpChecked event.
func TestCheckPlan_DriftEmitted(t *testing.T) {
	want := []spec.DriftDetail{
		{Field: "content", Current: "100 bytes", Desired: "200 bytes"},
	}

	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: driftCheckFn(want),
		ExecFn:  harness.PanicExecFn("must not execute in check mode"),
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:      "driftUnit",
			Desc:    "drift test",
			Actions: []spec.Action{harness.MkAction(opA)},
		},
	}

	rec := &harness.RecordingDisplayer{}
	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(ctx, rec), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, _, err = e.CheckPlan(diagnostic.NewCtx(ctx, rec), plan)
	if err != nil {
		t.Fatalf("CheckPlan: %v", err)
	}

	var found bool
	for _, c := range rec.Changes {
		if c.Phase != event.ChangePlanned {
			continue
		}
		d := c.Drift
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
		t.Fatal("no Change(Planned) event found")
	}
}

// CheckPlan on a satisfied op emits no drift detail.
func TestCheckPlan_SatisfiedNoDrift(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.PanicExecFn("must not execute"),
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:      "driftUnit",
			Desc:    "drift test",
			Actions: []spec.Action{harness.MkAction(opA)},
		},
	}

	rec := &harness.RecordingDisplayer{}
	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(ctx, rec), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, _, err = e.CheckPlan(diagnostic.NewCtx(ctx, rec), plan)
	if err != nil {
		t.Fatalf("CheckPlan: %v", err)
	}

	for _, c := range rec.Changes {
		if c.Phase == event.ChangePlanned {
			t.Fatal("satisfied op should have no Change(Planned) events")
		}
	}
}

// ExecutePlan does NOT emit drift (even on unsatisfied ops).
func TestExecutePlan_NoDrift(t *testing.T) {
	opA := &harness.FakeOp{
		Name:    "A",
		CheckFn: driftCheckFn([]spec.DriftDetail{{Field: "content", Current: "x", Desired: "y"}}),
		ExecFn:  harness.OkExecFn(true),
	}

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:      "driftUnit",
			Desc:    "drift test",
			Actions: []spec.Action{harness.MkAction(opA)},
		},
	}

	rec := &harness.RecordingDisplayer{}
	ctx := context.Background()
	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(ctx, rec), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(diagnostic.NewCtx(ctx, rec), plan)
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	for _, c := range rec.Changes {
		if c.Phase == event.ChangePlanned {
			t.Fatal("apply mode should not emit Change(Planned) events")
		}
	}
}
