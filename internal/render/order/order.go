// SPDX-License-Identifier: GPL-3.0-only

// Package order re-serializes a concurrent run's event stream into
// declaration order before it reaches an Output backend, so internal
// parallelism is invisible: the bytes a run emits match what serial
// execution would produce. It wraps any diagnostic.Output, so the CLI and a
// future --json backend both inherit ordering. See #433.
package order

import (
	"sort"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/spec"
)

// Sequencer buffers each step's Change/Result events and releases the whole
// step block to the wrapped Output in ascending step-index order, the moment a
// per-deploy cursor reaches a completed step. Diagnostics and progress are
// out-of-band and pass through immediately.
//
// Each deploy lane (keyed by event.StepRef.Deploy.Ordinal) has its own cursor.
// Within a lane, step indices are monotonic and unique across phases (hooks
// continue the index space, they don't reuse it), so the cursor orders the lane
// cleanly. Across lanes there is NO cursor: lanes release independently and
// interleave freely, which is the visible cross-deploy parallelism (#433).
//
// The Emitter serializes delivery, so RenderEvent / Flush / RenderSummary are
// never entered concurrently; the Sequencer holds no lock of its own.
type Sequencer struct {
	out   diagnostic.Output
	lanes map[int]*lane
}

// lane is one deploy's ordered buffer.
type lane struct {
	cursor  int
	pending map[int]*block
}

type block struct {
	changes []event.Change
	result  event.Result
	done    bool
}

var _ diagnostic.Output = (*Sequencer)(nil)

// New wraps out so the live event stream is released in declaration order,
// per deploy lane.
func New(out diagnostic.Output) *Sequencer {
	return &Sequencer{out: out, lanes: map[int]*lane{}}
}

func (s *Sequencer) laneFor(ord int) *lane {
	l := s.lanes[ord]
	if l == nil {
		l = &lane{pending: map[int]*block{}}
		s.lanes[ord] = l
	}
	return l
}

func (s *Sequencer) RenderEvent(e event.Event) {
	switch ev := e.(type) {
	case event.Change:
		l := s.laneFor(ev.Step.Deploy.Ordinal)
		s.collect(l, ev.Step.Index, func(b *block) { b.changes = append(b.changes, ev) })
	case event.Result:
		l := s.laneFor(ev.Step.Deploy.Ordinal)
		s.collect(l, ev.Step.Index, func(b *block) {
			b.result = ev
			b.done = true
		})
		s.advance(l)
	default:
		// Errors, warnings, info, progress: out-of-band, not part of the
		// ordered step record. Release immediately.
		s.out.RenderEvent(e)
	}
}

// collect routes a stream event into its step block within the lane.
func (s *Sequencer) collect(l *lane, idx int, mut func(*block)) {
	b := l.pending[idx]
	if b == nil {
		b = &block{}
		l.pending[idx] = b
	}
	mut(b)
}

// advance flushes completed blocks contiguously from the lane's cursor.
func (s *Sequencer) advance(l *lane) {
	for {
		b := l.pending[l.cursor]
		if b == nil || !b.done {
			return
		}
		s.flush(b)
		delete(l.pending, l.cursor)
		l.cursor++
	}
}

// flush replays one step's buffered events to the wrapped Output, drift
// before verdict, preserving within-step order.
func (s *Sequencer) flush(b *block) {
	for _, c := range b.changes {
		s.out.RenderEvent(c)
	}
	if b.done {
		s.out.RenderEvent(b.result)
	}
}

// drain flushes all pending blocks across all lanes, ordered by (lane ordinal,
// step index) for determinism. On abort this is where steps that finished in
// parallel past a failed/stuck cursor get reported honestly rather than dropped.
func (s *Sequencer) drain() {
	ords := make([]int, 0, len(s.lanes))
	for ord := range s.lanes {
		ords = append(ords, ord)
	}
	sort.Ints(ords)
	for _, ord := range ords {
		l := s.lanes[ord]
		idxs := make([]int, 0, len(l.pending))
		for i := range l.pending {
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)
		for _, i := range idxs {
			s.flush(l.pending[i])
			delete(l.pending, i)
		}
	}
}

// Flush releases everything still buffered, in declaration order per lane. It
// MUST run once at end of run, including the abort path where RenderSummary is
// skipped, or completed-but-buffered steps (the ones that finished in parallel
// past a failed/cancelled cursor) are stranded and lost. Idempotent: a second
// call drains an already-empty buffer.
func (s *Sequencer) Flush() {
	s.drain()
}

// RenderSummary marks end of run: drain the buffer, then emit the summary.
func (s *Sequencer) RenderSummary(rep result.Execution, checkOnly bool) {
	s.drain()
	s.out.RenderSummary(rep, checkOnly)
}

func (s *Sequencer) RenderPlan(p result.Plan)           { s.out.RenderPlan(p) }
func (s *Sequencer) RenderInspect(d result.Inspect)     { s.out.RenderInspect(d) }
func (s *Sequencer) RenderIndexAll(docs []spec.StepDoc) { s.out.RenderIndexAll(docs) }
func (s *Sequencer) RenderIndexStep(doc spec.StepDoc)   { s.out.RenderIndexStep(doc) }
func (s *Sequencer) RenderLegend()                      { s.out.RenderLegend() }
