// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"errors"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target/local"
	"scampi.dev/scampi/test/harness"
)

// Regression test for #330: when a step's op aborts on a non-Abort-impact
// diagnostic, the step must still surface a non-nil error so downstream
// steps don't get scheduled against the broken upstream.
func TestExecutePlan_OpAborted_NonAbortImpact_BlocksDownstream(t *testing.T) {
	actA := mkPromiserStep(nil, paths("/foo"),
		&harness.FakeOp{
			Name:    "A",
			CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
			ExecFn:  harness.DiagExecFn(signal.Error, diagnostic.ImpactNone),
		},
	)

	bOp := &harness.FakeOp{
		Name:    "B",
		CheckFn: harness.OkCheckFn(spec.CheckUnsatisfied),
		ExecFn:  harness.PanicExecFn("B must not run when A's op aborted"),
	}
	actB := mkPromiserStep(paths("/foo"), paths("/bar"), bOp)

	plan := spec.Plan{
		Deploy: spec.Deploy{
			ID:    "issue-330",
			Steps: []spec.Step{actA, actB},
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

	_, err = e.ExecutePlan(diagnostic.NewCtx(t.Context(), harness.NoopEmitter()), plan)
	if err == nil {
		t.Fatal("expected non-nil error after A's op aborted, got nil")
	}

	// Engine treats every non-AbortError/CancelledError as a BUG panic, so
	// any propagated step-abort error must satisfy errors.As(AbortError)
	// or errors.As(StepAbortedError) — either way, non-nil is the contract.
	var abort engine.AbortError
	var aborted engine.StepAbortedError
	if !errors.As(err, &abort) && !errors.As(err, &aborted) {
		t.Errorf("expected AbortError or StepAbortedError, got %T: %v", err, err)
	}

	if bOp.ExecCalls != 0 {
		t.Errorf("expected B not to execute, got %d call(s)", bOp.ExecCalls)
	}
}
