// SPDX-License-Identifier: GPL-3.0-only

package order_test

import (
	"fmt"
	"slices"
	"testing"

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
		r.log = append(r.log, fmt.Sprintf("change[%d]%s", ev.Step.Index, ev.DisplayID))
	case event.Result:
		r.log = append(r.log, fmt.Sprintf("result[%d]", ev.Step.Index))
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

func change(idx int, id string) event.Change {
	return event.Change{Step: event.StepRef{Index: idx}, DisplayID: id}
}

func res(idx int) event.Result {
	return event.Result{Step: event.StepRef{Index: idx}}
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

// An action's drift lines stay grouped with its verdict, in declaration order,
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

// A missing action (cancelled, no Result) doesn't wedge the buffer: completed
// later blocks drain at summary, in order.
func TestDrainOnSummary(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(0))
	s.RenderEvent(res(2)) // 1 never completes
	s.RenderSummary(result.Execution{}, true)

	assertLog(t, r.log, []string{"result[0]", "result[2]", "summary"})
}

// On abort, actions that finished in parallel past a stuck cursor are reported
// honestly at drain, in declaration order, not dropped.
func TestAbortDrainsCompletedPastCursor(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(0))
	s.RenderEvent(res(1))
	s.RenderEvent(res(3)) // action 2 cancelled; 3 finished in parallel
	s.RenderEvent(res(4))
	s.RenderSummary(result.Execution{}, false)

	assertLog(t, r.log, []string{
		"result[0]", "result[1]", "result[3]", "result[4]", "summary",
	})
}

// Diagnostics and progress are out-of-band: released immediately, never held
// behind buffered action blocks.
func TestDiagnosticsAndProgressPassThrough(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(event.Info{})
	s.RenderEvent(change(0, "a"))
	s.RenderEvent(event.Progress{})
	s.RenderEvent(res(0))

	assertLog(t, r.log, []string{"info", "progress", "change[0]a", "result[0]"})
}

// A reused index (hooks / multi-deploy) can't be ordered safely: drain what's
// held and pass everything through afterward, dropping nothing.
func TestIndexCollisionBypasses(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(0)) // releases [0], cursor -> 1
	s.RenderEvent(res(0)) // index reuse: bypass kicks in
	s.RenderEvent(res(0))

	assertLog(t, r.log, []string{"result[0]", "result[0]", "result[0]"})
}

// On abort the command returns before RenderSummary, so a deferred Flush is the
// only thing that drains completed-but-buffered actions. Without it they'd be
// stranded and lost.
func TestFlushDrainsWithoutSummary(t *testing.T) {
	r := &recorder{}
	s := order.New(r)
	s.RenderEvent(res(0))
	s.RenderEvent(res(2)) // action 1 cancelled; 2 finished in parallel
	s.Flush()

	assertLog(t, r.log, []string{"result[0]", "result[2]"})
}
