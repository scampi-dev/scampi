// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"errors"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target/local"
	"scampi.dev/scampi/test/harness"
)

// Regression test for #330: when an action's op aborts on a non-Abort-impact
// diagnostic, the action must still surface a non-nil error so downstream
// actions don't get scheduled against the broken upstream.
func TestExecutePlan_OpAborted_NonAbortImpact_BlocksDownstream(t *testing.T) {
	actA := mkPromiserAction(nil, paths("/foo"),
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
	actB := mkPromiserAction(paths("/foo"), paths("/bar"), bOp)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:      "issue-330",
			Actions: []spec.Action{actA, actB},
		},
	}

	cfg := spec.ResolvedConfig{
		Target: harness.MockTargetInstance(local.POSIXTarget{}),
	}
	e, err := engine.New(context.Background(), source.LocalPosixSource{}, cfg, harness.NoopEmitter{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	_, err = e.ExecutePlan(context.Background(), plan)
	if err == nil {
		t.Fatal("expected non-nil error after A's op aborted, got nil")
	}

	// Engine treats every non-AbortError/CancelledError as a BUG panic, so
	// any propagated action-abort error must satisfy errors.As(AbortError)
	// or errors.As(ActionAbortedError) — either way, non-nil is the contract.
	var abort engine.AbortError
	var aborted engine.ActionAbortedError
	if !errors.As(err, &abort) && !errors.As(err, &aborted) {
		t.Errorf("expected AbortError or ActionAbortedError, got %T: %v", err, err)
	}

	if bOp.ExecCalls != 0 {
		t.Errorf("expected B not to execute, got %d call(s)", bOp.ExecCalls)
	}
}
