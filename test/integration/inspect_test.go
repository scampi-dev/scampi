// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"bytes"
	"errors"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

func makeInspectEngine(t *testing.T, actions []spec.Action) *engine.Engine {
	t.Helper()

	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	steps := make([]spec.StepInstance, len(actions))
	for i, act := range actions {
		steps[i] = spec.StepInstance{
			Type: &harness.StubStepType{StepKind: act.Kind(), StepAction: act},
		}
	}

	cfg := spec.ResolvedConfig{
		Path:       "/test.scampi",
		DeployName: "test",
		TargetName: "local",
		Target:     harness.MockTargetInstance(tgt),
		Steps:      steps,
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), src, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	return e
}

func TestInspect_SingleOp(t *testing.T) {
	op := &harness.InspectableFakeOp{
		FakeOp: harness.FakeOp{
			Name: "copy-file", CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied), ExecFn: harness.OkExecFn(false),
		},
		Desired: []byte("desired content"),
		Current: []byte("current content"),
		Dest:    "/etc/app.conf",
	}
	act := harness.MkAction(&op.FakeOp)
	// Replace the plain FakeOp in the action with our inspectable one.
	act.AddOp(op)
	op.SetAction(act)

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	result, err := e.InspectDiffFile(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), "")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.DestPath != "/etc/app.conf" {
		t.Errorf("DestPath = %q, want %q", result.DestPath, "/etc/app.conf")
	}
	if !bytes.Equal(result.Desired, []byte("desired content")) {
		t.Errorf("Desired = %q, want %q", result.Desired, "desired content")
	}
	if !bytes.Equal(result.Current, []byte("current content")) {
		t.Errorf("Current = %q, want %q", result.Current, "current content")
	}
}

func TestInspect_NoInspectableOps(t *testing.T) {
	op := &harness.FakeOp{
		Name:    "chmod",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.OkExecFn(false),
	}
	act := harness.MkAction(op)

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	_, err := e.InspectDiffFile(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), "")
	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %v", err)
	}
}

func TestInspect_MultipleOps(t *testing.T) {
	op1 := &harness.InspectableFakeOp{
		FakeOp: harness.FakeOp{
			Name: "copy-a", CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied), ExecFn: harness.OkExecFn(false),
		},
		Desired: []byte("a"),
		Current: []byte("b"),
		Dest:    "/etc/a.conf",
	}
	op2 := &harness.InspectableFakeOp{
		FakeOp: harness.FakeOp{
			Name: "copy-b", CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied), ExecFn: harness.OkExecFn(false),
		},
		Desired: []byte("c"),
		Current: []byte("d"),
		Dest:    "/etc/b.conf",
	}
	act := harness.MkAction(&op1.FakeOp, &op2.FakeOp)
	// Replace FakeOps with inspectable ones
	act.AddOp(op1)
	act.AddOp(op2)
	op1.SetAction(act)
	op2.SetAction(act)

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	_, err := e.InspectDiffFile(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), "")
	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %v", err)
	}
}

func TestInspect_CurrentNotExist(t *testing.T) {
	op := &harness.InspectableFakeOp{
		FakeOp: harness.FakeOp{
			Name: "copy-new", CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied), ExecFn: harness.OkExecFn(false),
		},
		Desired: []byte("new file"),
		CurrErr: target.ErrNotExist,
		Dest:    "/etc/new.conf",
	}
	act := harness.MkAction(&op.FakeOp)
	act.AddOp(op)
	op.SetAction(act)

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	result, err := e.InspectDiffFile(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), "")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.Current != nil {
		t.Errorf("Current = %q, want nil", result.Current)
	}
	if !bytes.Equal(result.Desired, []byte("new file")) {
		t.Errorf("Desired = %q, want %q", result.Desired, "new file")
	}
}

func TestInspect_StepFilter(t *testing.T) {
	op1 := &harness.InspectableFakeOp{
		FakeOp: harness.FakeOp{
			Name: "copy-a", CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied), ExecFn: harness.OkExecFn(false),
		},
		Desired: []byte("a"),
		Current: []byte("b"),
		Dest:    "/etc/a.conf",
	}
	op2 := &harness.InspectableFakeOp{
		FakeOp: harness.FakeOp{
			Name: "copy-b", CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied), ExecFn: harness.OkExecFn(false),
		},
		Desired: []byte("c"),
		Current: []byte("d"),
		Dest:    "/etc/b.conf",
	}
	act := harness.MkAction(&op1.FakeOp, &op2.FakeOp)
	// Replace FakeOps with inspectable ones
	act.AddOp(op1)
	act.AddOp(op2)
	op1.SetAction(act)
	op2.SetAction(act)

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	// Without filter: multiple ops → abort.
	_, err := e.InspectDiffFile(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), "")
	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError without filter, got %v", err)
	}

	// With filter: narrows to one.
	result, err := e.InspectDiffFile(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), "a.conf")
	if err != nil {
		t.Fatalf("Inspect with filter: %v", err)
	}
	if result.DestPath != "/etc/a.conf" {
		t.Errorf("DestPath = %q, want %q", result.DestPath, "/etc/a.conf")
	}
}
