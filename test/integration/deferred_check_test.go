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

// fakeResourceStep wraps a harness.FakeStep with resource declarations for the
// step dependency graph and check-mode deferral.
type fakeResourceStep struct {
	harness.FakeStep
	requires []spec.Resource
	provides []spec.Resource
}

func (a *fakeResourceStep) Requires() []spec.Resource { return a.requires }
func (a *fakeResourceStep) Provides() []spec.Resource { return a.provides }

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

func mkResourceStep(requires, provides []spec.Resource, ops ...*harness.FakeOp) *fakeResourceStep {
	act := &fakeResourceStep{
		requires: requires,
		provides: provides,
	}
	for _, op := range ops {
		act.AddOp(op)
		op.SetStep(act)
	}
	return act
}

// Test_Check_DefersMissingDirWhenPathProvided verifies that check mode
// does not abort when a downstream op reports a missing directory that an
// upstream step will provide.
func Test_Check_DefersMissingDirWhenPathProvided(t *testing.T) {
	// dir step: check says "unsatisfied" (directory doesn't exist yet)
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkResourceStep(nil, paths("/foo"), dirOp)

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
	copyStep := mkResourceStep(paths("/foo"), paths("/foo/bar"), copyOp)

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

// Test_Check_DeferredPathNoProvideStillAborts verifies that a missing
// directory error still aborts when no upstream step provides the path.
func Test_Check_DeferredPathNoProvideStillAborts(t *testing.T) {
	copyOp := &harness.FakeOp{
		Name: "copy-file",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/nonexistent",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	copyStep := mkResourceStep(nil, paths("/nonexistent/bar"), copyOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-no-provide",
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
		t.Fatalf("CheckPlan must abort when no upstream step provides the path")
	}
}

// Test_Check_AbortsWhenUpstreamAlreadySatisfied verifies that a
// satisfied upstream step (CheckSatisfied, WouldChange=0) does NOT add
// its paths to the provided set, so a downstream missing-dir error still aborts.
func Test_Check_AbortsWhenUpstreamAlreadySatisfied(t *testing.T) {
	// dir step: already satisfied (directory exists)
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckSatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkResourceStep(nil, paths("/foo"), dirOp)

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
	copyStep := mkResourceStep(paths("/foo"), paths("/foo/bar"), copyOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-satisfied-no-provide",
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
		t.Fatalf("CheckPlan must abort: upstream is satisfied so path is not provided")
	}
}

// Test_Check_DeferredPathNonDeferrableErrorStillAborts verifies that abort
// errors that don't implement Deferrable are not deferred even when a
// matching provided path exists.
func Test_Check_DeferredPathNonDeferrableErrorStillAborts(t *testing.T) {
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkResourceStep(nil, paths("/foo"), dirOp)

	// This op returns a plain abort diagnostic (not Deferrable)
	abortOp := &harness.FakeOp{
		Name:    "abort-op",
		CheckFn: harness.DiagCheckFn(signal.Error, diagnostic.ImpactAbort),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	abortStep := mkResourceStep(paths("/foo"), paths("/foo/file"), abortOp)

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

// Test_Check_DefersMissingAncestorOfProvidedPath verifies that a provided path like
// /foo/bar also defers errors for /foo (MkdirAll creates ancestors).
func Test_Check_DefersMissingAncestorOfProvidedPath(t *testing.T) {
	// dir step provides /foo/bar (MkdirAll would create /foo too)
	dirOp := &harness.FakeOp{
		Name:    "ensure-dir",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkResourceStep(nil, paths("/foo/bar"), dirOp)

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
	copyStep := mkResourceStep(paths("/foo/bar"), paths("/foo/file"), copyOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-ancestor-provide",
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
		t.Fatalf("CheckPlan must not abort when ancestor path is provided, got: %v", err)
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
	dirStep := mkResourceStep(nil, paths("/foo"), dirOp)

	copyOp := &harness.FakeOp{
		Name: "copy-file",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, copy.CopyDestDirMissingError{
				Path: "/foo",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	copyStep := mkResourceStep(paths("/foo"), paths("/foo/bar"), copyOp)

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

// Test_Check_DefersUnknownUserWhenProvided verifies that check mode does
// not abort when a downstream op reports an unknown user that an upstream
// step will provide.
func Test_Check_DefersUnknownUserWhenProvided(t *testing.T) {
	// user step: check says "unsatisfied" (user doesn't exist yet)
	userOp := &harness.FakeOp{
		Name:    "ensure-user",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	userStep := mkResourceStep(nil, users("appd"), userOp)

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
	dirStep := mkResourceStep(users("appd"), paths("/opt/app"), dirOp)

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
		t.Fatalf("CheckPlan must not abort when user is provided, got: %v", err)
	}

	for i, ar := range rep.Steps {
		if ar.Summary.WouldChange == 0 {
			t.Errorf("step %d: expected WouldChange > 0, got %+v", i, ar.Summary)
		}
	}
}

// Test_Check_DefersUnknownGroupWhenProvided verifies the same for groups.
func Test_Check_DefersUnknownGroupWhenProvided(t *testing.T) {
	groupOp := &harness.FakeOp{
		Name:    "ensure-group",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("check mode must not execute"),
	}
	groupStep := mkResourceStep(nil, groups("appusers"), groupOp)

	dirOp := &harness.FakeOp{
		Name: "ensure-owner",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, sharedop.UnknownGroupError{
				Group: "appusers",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkResourceStep(groups("appusers"), paths("/opt/app"), dirOp)

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
		t.Fatalf("CheckPlan must not abort when group is provided, got: %v", err)
	}

	for i, ar := range rep.Steps {
		if ar.Summary.WouldChange == 0 {
			t.Errorf("step %d: expected WouldChange > 0, got %+v", i, ar.Summary)
		}
	}
}

// Test_Check_DeferredUserNoProvideStillAborts verifies that an unknown user
// error still aborts when no upstream step provides the user.
func Test_Check_DeferredUserNoProvideStillAborts(t *testing.T) {
	dirOp := &harness.FakeOp{
		Name: "ensure-owner",
		CheckFn: func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
			return spec.CheckUnsatisfied, nil, sharedop.UnknownUserError{
				User: "nobody-provided",
			}
		},
		ExecFn: harness.PanicExecFn("check mode must not execute"),
	}
	dirStep := mkResourceStep(nil, paths("/opt/app"), dirOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "test-user-no-provide",
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
		t.Fatalf("CheckPlan must abort when no upstream step provides the user")
	}
}
