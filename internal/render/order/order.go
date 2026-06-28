// SPDX-License-Identifier: GPL-3.0-only

// Package order re-serializes a concurrent run's event stream into
// declaration order before it reaches an Output backend, so internal
// parallelism is invisible: the bytes a run emits match what serial
// execution would produce. It wraps any diagnostic.Output, so the CLI and a
// future --json backend both inherit ordering. See #433.
package order

import (
	"sort"
	"sync"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/spec"
)

// Sequencer buffers each action's Change/Result events and releases the whole
// action block to the wrapped Output in ascending action-index order, the
// moment a cursor reaches a completed action. Diagnostics and progress are
// out-of-band and pass through immediately.
//
// The cursor keys on event.StepRef.Index, which is unique per deploy phase but
// reused across phases (hooks) and deploys. When a reused index is observed
// the Sequencer can no longer order safely, so it drains what it holds and
// bypasses from there on. Worst case is therefore today's unordered stream, a
// strict non-regression. Cross-phase/cross-deploy ordering needs a section key
// the events do not carry yet (#433, SP-C).
//
// The engine emits from parallel op/action goroutines, so RenderEvent is
// entered concurrently (the emitter does not serialize; only the sink below
// this decorator does). A mutex guards the buffer and cursor.
type Sequencer struct {
	out diagnostic.Output

	mu      sync.Mutex
	cursor  int
	pending map[int]*block
	bypass  bool
}

type block struct {
	changes []event.Change
	result  event.Result
	done    bool
}

var _ diagnostic.Output = (*Sequencer)(nil)

// New wraps out so the live event stream is released in declaration order.
func New(out diagnostic.Output) *Sequencer {
	return &Sequencer{out: out, pending: map[int]*block{}}
}

func (s *Sequencer) RenderEvent(e event.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.bypass {
		s.out.RenderEvent(e)
		return
	}

	switch ev := e.(type) {
	case event.Change:
		s.collect(ev.Step.Index, e, func(b *block) { b.changes = append(b.changes, ev) })
	case event.Result:
		s.collect(ev.Step.Index, e, func(b *block) {
			b.result = ev
			b.done = true
		})
		s.advance()
	default:
		// Errors, warnings, info, progress: out-of-band, not part of the
		// ordered action record. Release immediately.
		s.out.RenderEvent(e)
	}
}

// collect routes a stream event into its action block, or bails to bypass if
// the index was already released (section reuse).
func (s *Sequencer) collect(idx int, raw event.Event, mut func(*block)) {
	if idx < s.cursor {
		s.drainAndBypass(raw)
		return
	}
	b := s.pending[idx]
	if b == nil {
		b = &block{}
		s.pending[idx] = b
	}
	mut(b)
}

// advance flushes completed blocks contiguously from the cursor.
func (s *Sequencer) advance() {
	for {
		b := s.pending[s.cursor]
		if b == nil || !b.done {
			return
		}
		s.flush(b)
		delete(s.pending, s.cursor)
		s.cursor++
	}
}

// flush replays one action's buffered events to the wrapped Output, drift
// before verdict, preserving within-action order.
func (s *Sequencer) flush(b *block) {
	for _, c := range b.changes {
		s.out.RenderEvent(c)
	}
	if b.done {
		s.out.RenderEvent(b.result)
	}
}

// drainAndBypass releases everything held (in index order) and switches to
// pass-through. Used when the index space stops being monotonic.
func (s *Sequencer) drainAndBypass(raw event.Event) {
	s.drain()
	s.bypass = true
	s.out.RenderEvent(raw)
}

// drain flushes all pending blocks in ascending index order. On abort this is
// where actions that finished in parallel past a failed/stuck cursor get
// reported honestly rather than dropped.
func (s *Sequencer) drain() {
	idxs := make([]int, 0, len(s.pending))
	for i := range s.pending {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for _, i := range idxs {
		s.flush(s.pending[i])
		delete(s.pending, i)
	}
}

// Flush releases everything still buffered, in declaration order, then
// switches to pass-through. It MUST run once at end of run, including the abort
// path where RenderSummary is skipped, or completed-but-buffered actions (the
// ones that finished in parallel past a failed/cancelled cursor) are stranded
// and lost. Idempotent: a second call drains an already-empty buffer.
func (s *Sequencer) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drain()
	s.bypass = true
}

// RenderSummary marks end of run: drain the buffer, then emit the summary.
func (s *Sequencer) RenderSummary(rep result.Execution, checkOnly bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drain()
	s.out.RenderSummary(rep, checkOnly)
}

func (s *Sequencer) RenderPlan(p result.Plan)           { s.out.RenderPlan(p) }
func (s *Sequencer) RenderInspect(d result.Inspect)     { s.out.RenderInspect(d) }
func (s *Sequencer) RenderIndexAll(docs []spec.StepDoc) { s.out.RenderIndexAll(docs) }
func (s *Sequencer) RenderIndexStep(doc spec.StepDoc)   { s.out.RenderIndexStep(doc) }
func (s *Sequencer) RenderLegend()                      { s.out.RenderLegend() }
