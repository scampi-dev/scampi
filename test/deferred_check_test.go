// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/model"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/copy"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/local"
)

// fakePatherAction wraps a fakeAction with path declarations for the
// action dependency graph.
type fakePatherAction struct {
	fakeAction
	inputs  []string
	outputs []string
}

func (a *fakePatherAction) InputPaths() []string  { return a.inputs }
func (a *fakePatherAction) OutputPaths() []string { return a.outputs }

func mkPatherAction(inputs, outputs []string, ops ...*fakeOp) *fakePatherAction {
	act := &fakePatherAction{
		inputs:  inputs,
		outputs: outputs,
	}
	for _, op := range ops {
		act.ops = append(act.ops, op)
		op.action = act
	}
	return act
}

// TestCheck_DeferredPath_UpstreamPromisesDirectory verifies that check mode
// does not abort when a downstream op reports a missing directory that an
// upstream action has promised to create.
func TestCheck_DeferredPath_UpstreamPromisesDirectory(t *testing.T) {
	// dir action: check says "unsatisfied" (directory doesn't exist yet)
	dirOp := &fakeOp{
		name:    "ensure-dir",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  panicExecFn("check mode must not execute"),
	}
	dirAction := mkPatherAction(nil, []string{"/foo"}, dirOp)

	// copy action: check returns CopyDestDirMissingError for /foo
	copyOp := &fakeOp{
		name: "copy-file",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		execFn: panicExecFn("check mode must not execute"),
	}
	copyAction := mkPatherAction([]string{"/foo"}, []string{"/foo/bar"}, copyOp)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "test-deferred",
			Actions: []spec.Action{dirAction, copyAction},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(context.Background(), source.LocalPosixSource{}, cfg, noopEmitter{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	rep, err := e.CheckPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("CheckPlan must not return error when path is deferred, got: %v", err)
	}

	if len(rep.Actions) != 2 {
		t.Fatalf("expected 2 action reports, got %d", len(rep.Actions))
	}

	for i, ar := range rep.Actions {
		if ar.Summary.WouldChange == 0 {
			t.Errorf("action %d: expected WouldChange > 0, got %+v", i, ar.Summary)
		}
		if ar.Summary.Failed > 0 {
			t.Errorf("action %d: expected no failures, got Failed=%d", i, ar.Summary.Failed)
		}
		if ar.Summary.Aborted > 0 {
			t.Errorf("action %d: expected no aborts, got Aborted=%d", i, ar.Summary.Aborted)
		}
	}
}

// TestCheck_DeferredPath_NoPromise_StillAborts verifies that a missing
// directory error still aborts when no upstream action promises the path.
func TestCheck_DeferredPath_NoPromise_StillAborts(t *testing.T) {
	copyOp := &fakeOp{
		name: "copy-file",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/nonexistent",
			}
		},
		execFn: panicExecFn("check mode must not execute"),
	}
	copyAction := mkPatherAction(nil, []string{"/nonexistent/bar"}, copyOp)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "test-no-promise",
			Actions: []spec.Action{copyAction},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(context.Background(), source.LocalPosixSource{}, cfg, noopEmitter{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, err = e.CheckPlan(context.Background(), plan)
	if err == nil {
		t.Fatalf("CheckPlan must abort when no upstream action promises the path")
	}
}

// TestCheck_DeferredPath_UpstreamSatisfied_NoPromise verifies that a
// satisfied upstream action (CheckSatisfied, WouldChange=0) does NOT add
// its paths to the promised set, so a downstream missing-dir error still aborts.
func TestCheck_DeferredPath_UpstreamSatisfied_NoPromise(t *testing.T) {
	// dir action: already satisfied (directory exists)
	dirOp := &fakeOp{
		name:    "ensure-dir",
		checkFn: okCheckFn(spec.CheckSatisfied),
		execFn:  panicExecFn("check mode must not execute"),
	}
	dirAction := mkPatherAction(nil, []string{"/foo"}, dirOp)

	// copy action: missing dir error
	copyOp := &fakeOp{
		name: "copy-file",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		execFn: panicExecFn("check mode must not execute"),
	}
	copyAction := mkPatherAction([]string{"/foo"}, []string{"/foo/bar"}, copyOp)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "test-satisfied-no-promise",
			Actions: []spec.Action{dirAction, copyAction},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(context.Background(), source.LocalPosixSource{}, cfg, noopEmitter{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, err = e.CheckPlan(context.Background(), plan)
	if err == nil {
		t.Fatalf("CheckPlan must abort: upstream is satisfied so path is not promised")
	}
}

// TestCheck_DeferredPath_NonDeferrableError_StillAborts verifies that abort
// errors that don't implement DeferrablePath are not deferred even when a
// matching promised path exists.
func TestCheck_DeferredPath_NonDeferrableError_StillAborts(t *testing.T) {
	dirOp := &fakeOp{
		name:    "ensure-dir",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  panicExecFn("check mode must not execute"),
	}
	dirAction := mkPatherAction(nil, []string{"/foo"}, dirOp)

	// This op returns a plain abort diagnostic (not DeferrablePath)
	abortOp := &fakeOp{
		name:    "abort-op",
		checkFn: diagCheckFn(signal.Error, diagnostic.ImpactAbort),
		execFn:  panicExecFn("check mode must not execute"),
	}
	abortAction := mkPatherAction([]string{"/foo"}, []string{"/foo/file"}, abortOp)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "test-non-deferrable",
			Actions: []spec.Action{dirAction, abortAction},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(context.Background(), source.LocalPosixSource{}, cfg, noopEmitter{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, err = e.CheckPlan(context.Background(), plan)
	if err == nil {
		t.Fatalf("CheckPlan must abort for non-deferrable errors")
	}
}

// TestCheck_DeferredPath_AncestorPromise verifies that a promised path like
// /foo/bar also defers errors for /foo (MkdirAll creates ancestors).
func TestCheck_DeferredPath_AncestorPromise(t *testing.T) {
	// dir action promises /foo/bar (MkdirAll would create /foo too)
	dirOp := &fakeOp{
		name:    "ensure-dir",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  panicExecFn("check mode must not execute"),
	}
	dirAction := mkPatherAction(nil, []string{"/foo/bar"}, dirOp)

	// copy needs /foo to exist (parent of /foo/file)
	// Input depends on /foo/bar so the graph orders dir before copy.
	copyOp := &fakeOp{
		name: "copy-file",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		execFn: panicExecFn("check mode must not execute"),
	}
	copyAction := mkPatherAction([]string{"/foo/bar"}, []string{"/foo/file"}, copyOp)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "test-ancestor-promise",
			Actions: []spec.Action{dirAction, copyAction},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(context.Background(), source.LocalPosixSource{}, cfg, noopEmitter{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	rep, err := e.CheckPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("CheckPlan must not abort when ancestor path is promised, got: %v", err)
	}

	for i, ar := range rep.Actions {
		if ar.Summary.WouldChange == 0 {
			t.Errorf("action %d: expected WouldChange > 0", i)
		}
	}
}

// TestCheck_DeferredPath_OpOutcomeIsWouldChange verifies that deferred ops
// get OpWouldChange outcome (not OpAborted).
func TestCheck_DeferredPath_OpOutcomeIsWouldChange(t *testing.T) {
	dirOp := &fakeOp{
		name:    "ensure-dir",
		checkFn: okCheckFn(spec.CheckUnsatisfied),
		execFn:  panicExecFn("check mode must not execute"),
	}
	dirAction := mkPatherAction(nil, []string{"/foo"}, dirOp)

	copyOp := &fakeOp{
		name: "copy-file",
		checkFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		execFn: panicExecFn("check mode must not execute"),
	}
	copyAction := mkPatherAction([]string{"/foo"}, []string{"/foo/bar"}, copyOp)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "test-outcome",
			Actions: []spec.Action{dirAction, copyAction},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: mockTargetInstance(local.POSIXTarget{}),
	}

	e, err := engine.New(context.Background(), source.LocalPosixSource{}, cfg, noopEmitter{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	rep, err := e.CheckPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("CheckPlan: %v", err)
	}

	// The copy action's single op should be WouldChange
	copyReport := rep.Actions[1]
	if len(copyReport.Ops) != 1 {
		t.Fatalf("expected 1 op in copy action, got %d", len(copyReport.Ops))
	}

	if copyReport.Ops[0].Outcome != model.OpWouldChange {
		t.Errorf("deferred op outcome = %v, want OpWouldChange", copyReport.Ops[0].Outcome)
	}
	if copyReport.Ops[0].Err != nil {
		t.Errorf("deferred op should have nil error, got %v", copyReport.Ops[0].Err)
	}
}
