// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
)

func noopCtx(ctx context.Context) diagnostic.Ctx {
	return diagnostic.NewCtx(ctx, nil)
}

func TestRunPlansConcurrent_Empty(t *testing.T) {
	calls := 0
	err := runPlansConcurrent(noopCtx(t.Context()), nil,
		func(_ diagnostic.Ctx, _ event.DeployRef, _ spec.Config) error {
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
	resolved := []spec.Config{{DeployName: "a"}}
	var ran atomic.Int32
	err := runPlansConcurrent(noopCtx(t.Context()), resolved,
		func(_ diagnostic.Ctx, _ event.DeployRef, _ spec.Config) error {
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
	resolved := make([]spec.Config, n)
	for i := range resolved {
		resolved[i] = spec.Config{DeployName: "p"}
	}

	// Each worker increments inFlight, waits for all peers to arrive,
	// then exits. If runPlansConcurrent dispatched serially, inFlight
	// would never reach n and the test would time out.
	var inFlight atomic.Int32
	allArrived := make(chan struct{})

	err := runPlansConcurrent(noopCtx(t.Context()), resolved,
		func(_ diagnostic.Ctx, _ event.DeployRef, _ spec.Config) error {
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
	resolved := []spec.Config{
		{DeployName: "a"},
		{DeployName: "b"},
		{DeployName: "c"},
	}

	err := runPlansConcurrent(noopCtx(t.Context()), resolved,
		func(_ diagnostic.Ctx, _ event.DeployRef, res spec.Config) error {
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
	resolved := []spec.Config{
		{DeployName: "a"},
		{DeployName: "b"},
	}

	err := runPlansConcurrent(noopCtx(t.Context()), resolved,
		func(_ diagnostic.Ctx, _ event.DeployRef, res spec.Config) error {
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
	resolved := []spec.Config{
		{DeployName: "a"},
		{DeployName: "b"},
		{DeployName: "c"},
	}

	var (
		mu     sync.Mutex
		ranAll = map[string]bool{}
	)
	_ = runPlansConcurrent(noopCtx(t.Context()), resolved,
		func(_ diagnostic.Ctx, _ event.DeployRef, res spec.Config) error {
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

// mkLeveledConfigs builds a three-deploy set with a real resource edge:
// "producer" promises label x, "consumer"'s target consumes it, and "other"
// is independent. Levels: 0 = {producer, other}, 1 = {consumer}.
func mkLeveledConfigs() []spec.Config {
	producer := mkResolved("producer",
		fakeTargetKind{kind: "t"},
		fakeStaticStepKind{kind: "make.x", promises: []spec.Resource{spec.LabelResource("x")}},
	)
	other := mkResolved("other", fakeTargetKind{kind: "t"}, fakeStaticStepKind{kind: "noop"})
	consumer := mkResolved("consumer",
		fakeTargetKind{kind: "t", inputs: []spec.Resource{spec.LabelResource("x")}},
		fakeStaticStepKind{kind: "use.x"},
	)
	return []spec.Config{producer, other, consumer}
}

// A downstream level must not start until EVERY node in the upstream level
// has finished - not just its own producer. "other" signals completion only
// after a delay; if the level barrier leaked, "consumer" would enter while
// "other" is still running.
func TestRunPlansConcurrent_LevelBarrier(t *testing.T) {
	var producerDone, otherDone atomic.Bool

	err := runPlansConcurrent(noopCtx(t.Context()), mkLeveledConfigs(),
		func(_ diagnostic.Ctx, _ event.DeployRef, res spec.Config) error {
			switch res.DeployName {
			case "producer":
				producerDone.Store(true)
			case "other":
				time.Sleep(50 * time.Millisecond)
				otherDone.Store(true)
			case "consumer":
				if !producerDone.Load() {
					t.Error("consumer started before its producer finished")
				}
				if !otherDone.Load() {
					t.Error("consumer started before the whole upstream level finished")
				}
			}
			return nil
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Lane identity is assigned level-major, declaration order within a level,
// and the run-level constants (MaxNameWidth, RunTotalSteps) are the same on
// every ref.
func TestRunPlansConcurrent_AssignsLaneIdentity(t *testing.T) {
	var mu sync.Mutex
	refs := map[string]event.DeployRef{}

	err := runPlansConcurrent(noopCtx(t.Context()), mkLeveledConfigs(),
		func(_ diagnostic.Ctx, dr event.DeployRef, res spec.Config) error {
			mu.Lock()
			refs[res.DeployName] = dr
			mu.Unlock()
			return nil
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantOrd := map[string]int{"producer": 0, "other": 1, "consumer": 2}
	for name, want := range wantOrd {
		if got := refs[name].Ordinal; got != want {
			t.Errorf("%s ordinal = %d, want %d", name, got, want)
		}
	}
	const wantSteps = 3 // one step per deploy
	wantNameW := len("consumer")
	for name, dr := range refs {
		if dr.RunTotalSteps != wantSteps {
			t.Errorf("%s RunTotalSteps = %d, want %d", name, dr.RunTotalSteps, wantSteps)
		}
		if dr.MaxNameWidth != wantNameW {
			t.Errorf("%s MaxNameWidth = %d, want %d", name, dr.MaxNameWidth, wantNameW)
		}
		if dr.Name != name {
			t.Errorf("ref name = %q, want %q", dr.Name, name)
		}
	}
}

// An upstream failure skips downstream levels entirely (their producers
// couldn't satisfy them) while same-level siblings still run to completion.
func TestRunPlansConcurrent_UpstreamFailureSkipsDownstream(t *testing.T) {
	boom := errors.New("producer failed")
	var mu sync.Mutex
	ran := map[string]bool{}

	err := runPlansConcurrent(noopCtx(t.Context()), mkLeveledConfigs(),
		func(_ diagnostic.Ctx, _ event.DeployRef, res spec.Config) error {
			mu.Lock()
			ran[res.DeployName] = true
			mu.Unlock()
			if res.DeployName == "producer" {
				return boom
			}
			return nil
		})

	if !errors.Is(err, boom) {
		t.Errorf("expected producer error, got %v", err)
	}
	if !ran["other"] {
		t.Error("same-level sibling should run despite the failure")
	}
	if ran["consumer"] {
		t.Error("downstream level ran despite upstream failure")
	}
}

func TestRunPlansConcurrent_CtxCancellationPropagates(t *testing.T) {
	resolved := []spec.Config{
		{DeployName: "a"},
		{DeployName: "b"},
	}

	ctx, cancel := context.WithCancel(t.Context())
	// Signal from inside the worker as soon as one has entered its
	// blocking select - only then is it meaningful to cancel and
	// observe in-flight cancellation. The old `time.Sleep(20ms)`
	// approximated this and was flaky under load.
	started := make(chan struct{})
	var startOnce sync.Once

	go func() {
		<-started
		cancel()
	}()

	var observedCancel atomic.Bool
	_ = runPlansConcurrent(noopCtx(ctx), resolved, func(ctx diagnostic.Ctx, _ event.DeployRef, _ spec.Config) error {
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
