// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

func makeInspectEngine(t *testing.T, actions []spec.Action) *engine.Engine {
	t.Helper()

	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	steps := make([]spec.StepInstance, len(actions))
	for i, act := range actions {
		steps[i] = spec.StepInstance{
			Type: &stubStepType{kind: act.Kind(), action: act},
		}
	}

	cfg := spec.ResolvedConfig{
		Path:       "/test.scampi",
		DeployName: "test",
		TargetName: "local",
		Target:     mockTargetInstance(tgt),
		Steps:      steps,
	}

	e, err := engine.New(ctx, src, cfg, noopEmitter{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	return e
}

var ctx = context.Background()

func TestInspect_SingleOp(t *testing.T) {
	op := &inspectableFakeOp{
		fakeOp:  fakeOp{name: "copy-file", checkFn: okCheckFn(spec.CheckUnsatisfied), execFn: okExecFn(false)},
		desired: []byte("desired content"),
		current: []byte("current content"),
		dest:    "/etc/app.conf",
	}
	act := mkAction(&op.fakeOp)
	// Replace the plain fakeOp in the action with our inspectable one.
	act.ops = []spec.Op{op}
	op.action = act

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	result, err := e.InspectDiffFile(ctx, "")
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
	op := &fakeOp{
		name:    "chmod",
		checkFn: okCheckFn(spec.CheckSatisfied),
		execFn:  okExecFn(false),
	}
	act := mkAction(op)

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	_, err := e.InspectDiffFile(ctx, "")
	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %v", err)
	}
}

func TestInspect_MultipleOps(t *testing.T) {
	op1 := &inspectableFakeOp{
		fakeOp:  fakeOp{name: "copy-a", checkFn: okCheckFn(spec.CheckUnsatisfied), execFn: okExecFn(false)},
		desired: []byte("a"),
		current: []byte("b"),
		dest:    "/etc/a.conf",
	}
	op2 := &inspectableFakeOp{
		fakeOp:  fakeOp{name: "copy-b", checkFn: okCheckFn(spec.CheckUnsatisfied), execFn: okExecFn(false)},
		desired: []byte("c"),
		current: []byte("d"),
		dest:    "/etc/b.conf",
	}
	act := &fakeAction{ops: []spec.Op{op1, op2}}
	op1.action = act
	op2.action = act

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	_, err := e.InspectDiffFile(ctx, "")
	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %v", err)
	}
}

func TestInspect_CurrentNotExist(t *testing.T) {
	op := &inspectableFakeOp{
		fakeOp:  fakeOp{name: "copy-new", checkFn: okCheckFn(spec.CheckUnsatisfied), execFn: okExecFn(false)},
		desired: []byte("new file"),
		currErr: target.ErrNotExist,
		dest:    "/etc/new.conf",
	}
	act := mkAction(&op.fakeOp)
	act.ops = []spec.Op{op}
	op.action = act

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	result, err := e.InspectDiffFile(ctx, "")
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
	op1 := &inspectableFakeOp{
		fakeOp:  fakeOp{name: "copy-a", checkFn: okCheckFn(spec.CheckUnsatisfied), execFn: okExecFn(false)},
		desired: []byte("a"),
		current: []byte("b"),
		dest:    "/etc/a.conf",
	}
	op2 := &inspectableFakeOp{
		fakeOp:  fakeOp{name: "copy-b", checkFn: okCheckFn(spec.CheckUnsatisfied), execFn: okExecFn(false)},
		desired: []byte("c"),
		current: []byte("d"),
		dest:    "/etc/b.conf",
	}
	act := &fakeAction{ops: []spec.Op{op1, op2}}
	op1.action = act
	op2.action = act

	e := makeInspectEngine(t, []spec.Action{act})
	defer e.Close()

	// Without filter: multiple ops → abort.
	_, err := e.InspectDiffFile(ctx, "")
	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError without filter, got %v", err)
	}

	// With filter: narrows to one.
	result, err := e.InspectDiffFile(ctx, "a.conf")
	if err != nil {
		t.Fatalf("Inspect with filter: %v", err)
	}
	if result.DestPath != "/etc/a.conf" {
		t.Errorf("DestPath = %q, want %q", result.DestPath, "/etc/a.conf")
	}
}
