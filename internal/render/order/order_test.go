// SPDX-License-Identifier: GPL-3.0-only

package order_test

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/render/order"
	"scampi.dev/scampi/internal/spec"
)

// recorder is a diagnostic.Output that records the order things are released.
type recorder struct{ log []string }

func (r *recorder) RenderEvent(e event.Event) {
	switch ev := e.(type) {
	case event.Change:
		r.log = append(r.log, fmt.Sprintf("change[%s%d]%s", lane(ev.Step), ev.Step.Index, ev.DisplayID))
	case event.Result:
		r.log = append(r.log, fmt.Sprintf("result[%s%d]", lane(ev.Step), ev.Step.Index))
	case event.Progress:
		r.log = append(r.log, "progress")
	case event.Info:
		r.log = append(r.log, "info")
	default:
		r.log = append(r.log, "other")
	}
}

func (r *recorder) RenderSummary(result.Execution, bool) { r.log = append(r.log, "summary") }
func (r *recorder) RenderPlan(result.Plan)               {}
func (r *recorder) RenderInspect(result.Inspect)         {}
func (r *recorder) RenderIndexAll([]spec.StepDoc)        {}
func (r *recorder) RenderIndexStep(spec.StepDoc)         {}
func (r *recorder) RenderLegend()                        {}

// lane returns a "name:" prefix for named (multi-deploy) lanes, "" otherwise,
// so single-lane test logs stay terse.
func lane(s event.StepRef) string {
	if s.Deploy.Name == "" {
		return ""
	}
	return s.Deploy.Name + ":"
}

func change(idx int, id string) event.Change {
	return event.Change{Step: event.StepRef{Index: idx}, DisplayID: id}
}

func res(idx int) event.Result {
	return event.Result{Step: event.StepRef{Index: idx}}
}

// resIn builds a Result in a named deploy lane (name + ordinal).
func resIn(name string, ord, idx int) event.Result {
	return event.Result{Step: event.StepRef{
		Deploy: event.DeployRef{Name: name, Ordinal: ord},
		Index:  idx,
	}}
}

func assertLog(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("release order mismatch\n got: %v\nwant: %v", got, want)
	}
}

// In declaration order, each Result releases its block immediately.
func TestInOrder(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(change(0, "a"))
	s.RenderEvent(res(0))
	s.RenderEvent(change(1, "b"))
	s.RenderEvent(res(1))

	assertLog(t, r.log, []string{"change[0]a", "result[0]", "change[1]b", "result[1]"})
}

// Out-of-order completion is re-serialized: nothing for an index releases
// until the cursor reaches it.
func TestScrambledCompletionReleasesInOrder(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(2))
	s.RenderEvent(res(0))
	s.RenderEvent(res(1))

	assertLog(t, r.log, []string{"result[0]", "result[1]", "result[2]"})
}

// A step's drift lines stay grouped with its verdict, in declaration order,
// regardless of interleaved arrival.
func TestChangesStayGroupedAndOrdered(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(change(1, "x"))
	s.RenderEvent(change(0, "y"))
	s.RenderEvent(change(0, "z"))
	s.RenderEvent(res(1))
	s.RenderEvent(res(0))

	assertLog(t, r.log, []string{
		"change[0]y", "change[0]z", "result[0]",
		"change[1]x", "result[1]",
	})
}

// A missing step (cancelled, no Result) doesn't wedge the buffer: completed
// later blocks drain at summary, in order.
func TestDrainOnSummary(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(0))
	s.RenderEvent(res(2)) // 1 never completes
	s.RenderSummary(result.Execution{}, true)

	assertLog(t, r.log, []string{"result[0]", "result[2]", "summary"})
}

// On abort, steps that finished in parallel past a stuck cursor are reported
// honestly at drain, in declaration order, not dropped.
func TestAbortDrainsCompletedPastCursor(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(0))
	s.RenderEvent(res(1))
	s.RenderEvent(res(3)) // step 2 cancelled; 3 finished in parallel
	s.RenderEvent(res(4))
	s.RenderSummary(result.Execution{}, false)

	assertLog(t, r.log, []string{
		"result[0]", "result[1]", "result[3]", "result[4]", "summary",
	})
}

// Diagnostics and progress are out-of-band: released immediately, never held
// behind buffered step blocks.
func TestDiagnosticsAndProgressPassThrough(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(event.Info{})
	s.RenderEvent(change(0, "a"))
	s.RenderEvent(event.Progress{})
	s.RenderEvent(res(0))

	assertLog(t, r.log, []string{"info", "progress", "change[0]a", "result[0]"})
}

// Each deploy lane orders independently and interleaves freely: a fast lane
// streams its results without waiting on a slow sibling, while each lane stays
// internally in declaration order. This is the visible cross-deploy parallelism.
func TestPerDeployLanesOrderIndependently(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(resIn("dns", 1, 0)) // dns lane cursor 0 -> releases now
	s.RenderEvent(resIn("web", 0, 1)) // web 1 done, but web cursor 0 -> buffered
	s.RenderEvent(resIn("dns", 1, 1)) // dns cursor 1 -> releases now (web still buffered)
	s.RenderEvent(resIn("web", 0, 0)) // web 0 -> releases 0 then the buffered 1

	assertLog(t, r.log, []string{
		"result[dns:0]", "result[dns:1]", "result[web:0]", "result[web:1]",
	})
}

// Hooks continue a lane's index space (N, N+1, ...) rather than reusing it, so
// the per-lane cursor keeps ordering across the main and hook phases.
func TestHookIndicesContinueLane(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(0))
	s.RenderEvent(res(1))
	s.RenderEvent(res(2)) // hook step, continues the index space
	assertLog(t, r.log, []string{"result[0]", "result[1]", "result[2]"})
}

// On abort the command returns before RenderSummary, so a deferred Flush is the
// only thing that drains completed-but-buffered steps. Without it they'd be
// stranded and lost.
func TestFlushDrainsWithoutSummary(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(0))
	s.RenderEvent(res(2)) // step 1 cancelled; 2 finished in parallel
	s.Flush()

	assertLog(t, r.log, []string{"result[0]", "result[2]"})
}

// The engine emits from parallel goroutines, but the Emitter serializes
// delivery, so the Sequencer (and the output below it) see one call at a time
// and hold no locks. Run under -race: concurrent producers go through the
// emitter, never the Sequencer directly.
func TestConcurrentEmitIsRaceFree(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	em := diagnostic.NewEmitter(diagnostic.Policy{}, s)

	const n = 64
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			em.Emit(change(idx, "f"))
			em.Emit(res(idx))
		}(i)
	}
	wg.Wait()
	s.Flush()

	got := 0
	for _, l := range r.log {
		if strings.HasPrefix(l, "result[") {
			got++
		}
	}
	if got != n {
		t.Fatalf("released %d results, want %d", got, n)
	}
}
