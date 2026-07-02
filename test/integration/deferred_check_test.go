// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/copy"
	"scampi.dev/scampi/internal/step/sharedop"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/local"
	"scampi.dev/scampi/test/harness"
)

// fakePromiserStep wraps a harness.FakeStep with resource declarations for the
// step dependency graph and promise system.
type fakePromiserStep struct {
	harness.FakeStep
	inputs   []spec.Resource
	promises []spec.Resource
}

func (a *fakePromiserStep) Inputs() []spec.Resource   { return a.inputs }
func (a *fakePromiserStep) Promises() []spec.Resource { return a.promises }

func paths(s ...string) []spec.Resource {
	r := make([]spec.Resource, len(s))
	for i, p := range s {
		r[i] = spec.PathResource(p)
	}
	return r
}

func users(s ...string) []spec.Resource {
	r := make([]spec.Resource, len(s))
	for i, u := range s {
		r[i] = spec.UserResource(u)
	}
	return r
}

func groups(s ...string) []spec.Resource {
	r := make([]spec.Resource, len(s))
	for i, g := range s {
		r[i] = spec.GroupResource(g)
	}
	return r
}

func mkPromiserStep(inputs, promises []spec.Resource, ops ...*harness.FakeOp) *fakePromiserStep {
	act := &fakePromiserStep{
		inputs:   inputs,
		promises: promises,
	}
	for _, op := range ops {
		act.AddOp(op)
		op.SetStep(act)
	}
	return act
}

// Test_Check_DeferredPathUpstreamPromisesDirectory verifies that check mode
// does not abort when a downstream op reports a missing directory that an
// upstream step has promised to create.
func Test_Check_DeferredPathUpstreamPromisesDirectory(t *testing.T) {
	// dir step: check says "unsatisfied" (directory doesn't exist yet)
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkPromiserStep(nil, paths("/foo"), dirOp)

	// copy step: check returns CopyDestDirMissingError for /foo
	copyOp := &harness.FakeOp{
		Name: "copy-file",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	copyStep := mkPromiserStep(paths("/foo"), paths("/foo/bar"), copyOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-deferred",
			Steps: []spec.Step{dirStep, copyStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	rep, _, err := e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err != nil {
		t.Fatalf("CheckPlan must not return error when path is deferred, got: %v", err)
	}

	if len(rep.Steps) != 2 {
		t.Fatalf("expected 2 step reports, got %d", len(rep.Steps))
	}

	for i, ar := range rep.Steps {
		if ar.Summary.WouldChange == 0 {
			t.Errorf("step %d: expected WouldChange > 0, got %+v", i, ar.Summary)
		}
		if ar.Summary.Failed > 0 {
			t.Errorf("step %d: expected no failures, got Failed=%d", i, ar.Summary.Failed)
		}
		if ar.Summary.Aborted > 0 {
			t.Errorf("step %d: expected no aborts, got Aborted=%d", i, ar.Summary.Aborted)
		}
	}
}

// Test_Check_DeferredPathNoPromiseStillAborts verifies that a missing
// directory error still aborts when no upstream step promises the path.
func Test_Check_DeferredPathNoPromiseStillAborts(t *testing.T) {
	copyOp := &harness.FakeOp{
		Name: "copy-file",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/nonexistent",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	copyStep := mkPromiserStep(nil, paths("/nonexistent/bar"), copyOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-no-promise",
			Steps: []spec.Step{copyStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, _, err = e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err == nil {
		t.Fatalf("CheckPlan must abort when no upstream step promises the path")
	}
}

// Test_Check_DeferredPathUpstreamSatisfiedNoPromise verifies that a
// satisfied upstream step (CheckSatisfied, WouldChange=0) does NOT add
// its paths to the promised set, so a downstream missing-dir error still aborts.
func Test_Check_DeferredPathUpstreamSatisfiedNoPromise(t *testing.T) {
	// dir step: already satisfied (directory exists)
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkPromiserStep(nil, paths("/foo"), dirOp)

	// copy step: missing dir error
	copyOp := &harness.FakeOp{
		Name: "copy-file",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	copyStep := mkPromiserStep(paths("/foo"), paths("/foo/bar"), copyOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-satisfied-no-promise",
			Steps: []spec.Step{dirStep, copyStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, _, err = e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err == nil {
		t.Fatalf("CheckPlan must abort: upstream is satisfied so path is not promised")
	}
}

// Test_Check_DeferredPathNonDeferrableErrorStillAborts verifies that abort
// errors that don't implement Deferrable are not deferred even when a
// matching promised path exists.
func Test_Check_DeferredPathNonDeferrableErrorStillAborts(t *testing.T) {
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkPromiserStep(nil, paths("/foo"), dirOp)

	// This op returns a plain abort diagnostic (not Deferrable)
	abortOp := &harness.FakeOp{
		Name:    "abort-op",
		CheckFn: harness.DiagCheckFn(signal.Error, diagnostic.ImpactAbort),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	abortStep := mkPromiserStep(paths("/foo"), paths("/foo/file"), abortOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-non-deferrable",
			Steps: []spec.Step{dirStep, abortStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, _, err = e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err == nil {
		t.Fatalf("CheckPlan must abort for non-deferrable errors")
	}
}

// Test_Check_DeferredPathAncestorPromise verifies that a promised path like
// /foo/bar also defers errors for /foo (MkdirAll creates ancestors).
func Test_Check_DeferredPathAncestorPromise(t *testing.T) {
	// dir step promises /foo/bar (MkdirAll would create /foo too)
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkPromiserStep(nil, paths("/foo/bar"), dirOp)

	// copy needs /foo to exist (parent of /foo/file)
	// Input depends on /foo/bar so the graph orders dir before copy.
	copyOp := &harness.FakeOp{
		Name: "copy-file",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	copyStep := mkPromiserStep(paths("/foo/bar"), paths("/foo/file"), copyOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-ancestor-promise",
			Steps: []spec.Step{dirStep, copyStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	rep, _, err := e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err != nil {
		t.Fatalf("CheckPlan must not abort when ancestor path is promised, got: %v", err)
	}

	for i, ar := range rep.Steps {
		if ar.Summary.WouldChange == 0 {
			t.Errorf("step %d: expected WouldChange > 0", i)
		}
	}
}

// Test_Check_DeferredPathOpOutcomeIsWouldChange verifies that deferred ops
// get OpWouldChange outcome (not OpAborted).
func Test_Check_DeferredPathOpOutcomeIsWouldChange(t *testing.T) {
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkPromiserStep(nil, paths("/foo"), dirOp)

	copyOp := &harness.FakeOp{
		Name: "copy-file",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	copyStep := mkPromiserStep(paths("/foo"), paths("/foo/bar"), copyOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-outcome",
			Steps: []spec.Step{dirStep, copyStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	rep, _, err := e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err != nil {
		t.Fatalf("CheckPlan: %v", err)
	}

	// The copy step's single op should be WouldChange
	copyReport := rep.Steps[1]
	if len(copyReport.Ops) != 1 {
		t.Fatalf("expected 1 op in copy step, got %d", len(copyReport.Ops))
	}

	if copyReport.Ops[0].Outcome != result.OpWouldChange {
		t.Errorf("deferred op outcome = %v, want OpWouldChange", copyReport.Ops[0].Outcome)
	}
	if copyReport.Ops[0].Err != nil {
		t.Errorf("deferred op should have nil error, got %v", copyReport.Ops[0].Err)
	}
}

// Test_Check_DeferredUserUpstreamPromisesUser verifies that check mode does
// not abort when a downstream op reports an unknown user that an upstream
// step has promised to create.
func Test_Check_DeferredUserUpstreamPromisesUser(t *testing.T) {
	// user step: check says "unsatisfied" (user doesn't exist yet)
	userOp := &harness.FakeOp{
		Name:    "ensure-user",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	userStep := mkPromiserStep(nil, users("appd"), userOp)

	// dir step: check returns UnknownUserError for appd
	dirOp := &harness.FakeOp{
		Name: "ensure-owner",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, sharedop.UnknownUserError{
				User: "appd",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkPromiserStep(users("appd"), paths("/opt/app"), dirOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-deferred-user",
			Steps: []spec.Step{userStep, dirStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	rep, _, err := e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err != nil {
		t.Fatalf("CheckPlan must not abort when user is promised, got: %v", err)
	}

	for i, ar := range rep.Steps {
		if ar.Summary.WouldChange == 0 {
			t.Errorf("step %d: expected WouldChange > 0, got %+v", i, ar.Summary)
		}
	}
}

// Test_Check_DeferredGroupUpstreamPromisesGroup verifies the same for groups.
func Test_Check_DeferredGroupUpstreamPromisesGroup(t *testing.T) {
	groupOp := &harness.FakeOp{
		Name:    "ensure-group",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	groupStep := mkPromiserStep(nil, groups("appusers"), groupOp)

	dirOp := &harness.FakeOp{
		Name: "ensure-owner",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, sharedop.UnknownGroupError{
				Group: "appusers",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkPromiserStep(groups("appusers"), paths("/opt/app"), dirOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-deferred-group",
			Steps: []spec.Step{groupStep, dirStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	rep, _, err := e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err != nil {
		t.Fatalf("CheckPlan must not abort when group is promised, got: %v", err)
	}

	for i, ar := range rep.Steps {
		if ar.Summary.WouldChange == 0 {
			t.Errorf("step %d: expected WouldChange > 0, got %+v", i, ar.Summary)
		}
	}
}

// Test_Check_DeferredUserNoPromiseStillAborts verifies that an unknown user
// error still aborts when no upstream step promises the user.
func Test_Check_DeferredUserNoPromiseStillAborts(t *testing.T) {
	dirOp := &harness.FakeOp{
		Name: "ensure-owner",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, sharedop.UnknownUserError{
				User: "nobody-promised",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkPromiserStep(nil, paths("/opt/app"), dirOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-user-no-promise",
			Steps: []spec.Step{dirStep},
		},
	}

	cfg := spec.Config{
		Target: harness.MockDeclaredTarget(local.POSIXTarget{}),
	}

	e, err := engine.New(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), source.LocalPosixSource{}, cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, _, err = e.CheckPlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err == nil {
		t.Fatalf("CheckPlan must abort when no upstream step promises the user")
	}
}
