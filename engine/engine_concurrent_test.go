// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"scampi.dev/scampi/spec"
)

func TestRunPlansConcurrent_Empty(t *testing.T) {
	calls := 0
	err := runPlansConcurrent(t.Context(), nil, nil, func(_ context.Context, _ spec.ResolvedConfig) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Errorf("got %d calls, want 0", calls)
	}
}

func TestRunPlansConcurrent_Single(t *testing.T) {
	resolved := []spec.ResolvedConfig{{DeployName: "a"}}
	var ran atomic.Int32
	err := runPlansConcurrent(t.Context(), nil, resolved, func(_ context.Context, _ spec.ResolvedConfig) error {
		ran.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := ran.Load(); got != 1 {
		t.Errorf("got %d work calls, want 1", got)
	}
}

func TestRunPlansConcurrent_RunsInParallel(t *testing.T) {
	const n = 4
	resolved := make([]spec.ResolvedConfig, n)
	for i := range resolved {
		resolved[i] = spec.ResolvedConfig{DeployName: "p"}
	}

	// Each worker increments inFlight, waits for all peers to arrive,
	// then exits. If runPlansConcurrent dispatched serially, inFlight
	// would never reach n and the test would time out.
	var inFlight atomic.Int32
	allArrived := make(chan struct{})

	err := runPlansConcurrent(t.Context(), nil, resolved, func(_ context.Context, _ spec.ResolvedConfig) error {
		if inFlight.Add(1) == n {
			close(allArrived)
		}
		select {
		case <-allArrived:
		case <-time.After(2 * time.Second):
			t.Errorf("worker timed out waiting for peers (serial execution?)")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := inFlight.Load(); got != n {
		t.Errorf("inFlight = %d, want %d", got, n)
	}
}

func TestRunPlansConcurrent_AggregatesErrors(t *testing.T) {
	errA := errors.New("plan a failed")
	errB := errors.New("plan b failed")
	resolved := []spec.ResolvedConfig{
		{DeployName: "a"},
		{DeployName: "b"},
		{DeployName: "c"},
	}

	err := runPlansConcurrent(t.Context(), nil, resolved, func(_ context.Context, res spec.ResolvedConfig) error {
		switch res.DeployName {
		case "a":
			return errA
		case "b":
			return errB
		}
		return nil
	})

	var abort AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T (%v)", err, err)
	}
	if len(abort.Causes) != 2 {
		t.Fatalf("expected 2 causes, got %d", len(abort.Causes))
	}
	seen := map[error]bool{}
	for _, c := range abort.Causes {
		seen[c] = true
	}
	if !seen[errA] || !seen[errB] {
		t.Errorf("missing expected causes: got %v", abort.Causes)
	}
}

func TestRunPlansConcurrent_SingleErrorUnwrapped(t *testing.T) {
	target := errors.New("only one fails")
	resolved := []spec.ResolvedConfig{
		{DeployName: "a"},
		{DeployName: "b"},
	}

	err := runPlansConcurrent(t.Context(), nil, resolved, func(_ context.Context, res spec.ResolvedConfig) error {
		if res.DeployName == "b" {
			return target
		}
		return nil
	})

	if !errors.Is(err, target) {
		t.Errorf("expected single error returned unwrapped, got %T (%v)", err, err)
	}
}

func TestRunPlansConcurrent_SiblingsRunDespiteFailure(t *testing.T) {
	resolved := []spec.ResolvedConfig{
		{DeployName: "a"},
		{DeployName: "b"},
		{DeployName: "c"},
	}

	var (
		mu     sync.Mutex
		ranAll = map[string]bool{}
	)
	_ = runPlansConcurrent(t.Context(), nil, resolved, func(_ context.Context, res spec.ResolvedConfig) error {
		mu.Lock()
		ranAll[res.DeployName] = true
		mu.Unlock()
		if res.DeployName == "a" {
			return errors.New("a failed")
		}
		return nil
	})

	for _, name := range []string{"a", "b", "c"} {
		if !ranAll[name] {
			t.Errorf("plan %q did not run despite sibling failure", name)
		}
	}
}

func TestRunPlansConcurrent_CtxCancellationPropagates(t *testing.T) {
	resolved := []spec.ResolvedConfig{
		{DeployName: "a"},
		{DeployName: "b"},
	}

	ctx, cancel := context.WithCancel(t.Context())
	// Signal from inside the worker as soon as one has entered its
	// blocking select — only then is it meaningful to cancel and
	// observe in-flight cancellation. The old `time.Sleep(20ms)`
	// approximated this and was flaky under load.
	started := make(chan struct{})
	var startOnce sync.Once

	go func() {
		<-started
		cancel()
	}()

	var observedCancel atomic.Bool
	_ = runPlansConcurrent(ctx, nil, resolved, func(ctx context.Context, _ spec.ResolvedConfig) error {
		startOnce.Do(func() { close(started) })
		select {
		case <-ctx.Done():
			observedCancel.Store(true)
			return ctx.Err()
		case <-time.After(2 * time.Second):
			return nil
		}
	})

	if !observedCancel.Load() {
		t.Error("plan workers did not observe context cancellation")
	}
}
