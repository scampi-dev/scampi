// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"sync"
	"time"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/render/order"
	"scampi.dev/scampi/internal/spec"
)

const (
	// streamBuffer is the event channel depth: a load-backstop so the engine
	// never blocks on rendering. A run emitting faster than the terminal can
	// repaint queues here; only a sustained 1024-event backlog stalls a producer.
	streamBuffer = 1024
	// spinnerInterval paces live-region repaints during event silence (spinner
	// motion, elapsed times) so a slow step never looks hung.
	spinnerInterval = 100 * time.Millisecond
)

// streamSink is the check/apply stream surface. RenderEvent just drops the event
// into a buffered channel, so the engine is never blocked by rendering and never
// has to reason about concurrency: emitting is always safe from any goroutine. A
// single consumer goroutine drains the channel and is the only thing that touches
// the CLI, sink, Sequencer, and in-flight tracker, so none of them need locks.
// The channel is the serialization point.
type streamSink struct {
	cli      *CLI
	seq      *order.Sequencer
	inflight *inflight

	events chan event.Event
	done   chan struct{}
	once   sync.Once
	frame  int
}

var _ Stream = (*streamSink)(nil)

// Stream is the streaming output surface: an Output plus Flush, which stops the
// consumer goroutine and wipes the live region (the abort-path drain).
type Stream interface {
	diagnostic.Output
	Flush()
}

// newStreamSink builds the sink without starting the consumer goroutine, so
// tests can drive handle/finish synchronously and assert the output.
func newStreamSink(c *CLI) *streamSink {
	return &streamSink{
		cli:      c,
		seq:      order.New(c),
		inflight: newInflight(),
		events:   make(chan event.Event, streamBuffer),
		done:     make(chan struct{}),
	}
}

// NewStream wraps a CLI in the channel-backed stream surface and starts its
// consumer goroutine. Call Flush (or RenderSummary) to stop it.
func NewStream(c *CLI) Stream {
	s := newStreamSink(c)
	go s.run()
	return s
}

// RenderEvent hands the event to the consumer without blocking (until the 1024
// buffer fills). Safe from any goroutine.
func (s *streamSink) RenderEvent(e event.Event) { s.events <- e }

func (s *streamSink) run() {
	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()
	for {
		select {
		case e, ok := <-s.events:
			if !ok {
				s.finish()
				close(s.done)
				return
			}
			s.handle(e)
		case <-ticker.C:
			if s.inflight.anyRunning() {
				s.frame++
				s.repaint()
			}
		}
	}
}

func (s *streamSink) handle(e event.Event) {
	// Real-time in-flight tracking from the raw event (begin/finish order).
	switch ev := e.(type) {
	case event.Begin:
		s.inflight.begin(ev.Step, s.cli.now())
	case event.Result:
		s.inflight.finish(ev.Step)
	}
	// Durable scrollback: per-deploy ordering, released blocks go to the sink.
	s.seq.RenderEvent(e)
	s.repaint()
}

// finish drains buffered durable blocks in order and wipes the live region.
// Called by the consumer on channel close, and by tests after driving handle.
func (s *streamSink) finish() {
	s.seq.Flush()
	s.cli.sink.clearRegion()
}

func (s *streamSink) repaint() {
	s.cli.sink.setRegion(s.cli.regionLines(s.inflight, s.frame))
}

// RenderSummary stops the consumer (draining and wiping the region) then prints
// the summary synchronously, with no region in play.
func (s *streamSink) RenderSummary(rep result.Execution, checkOnly bool) {
	s.stop()
	s.cli.RenderSummary(rep, checkOnly)
}

// Flush stops the consumer. Used on the abort path where RenderSummary is
// skipped; the deferred call drains completed-but-buffered blocks and wipes the
// region. Idempotent.
func (s *streamSink) Flush() { s.stop() }

func (s *streamSink) stop() {
	s.once.Do(func() {
		close(s.events)
		<-s.done
	})
}

// One-shot value methods delegate to the CLI. Not used on the stream surface
// (plan/inspect/index build the synchronous sink), but Output requires them.
func (s *streamSink) RenderPlan(p result.Plan)           { s.cli.RenderPlan(p) }
func (s *streamSink) RenderInspect(d result.Inspect)     { s.cli.RenderInspect(d) }
func (s *streamSink) RenderIndexAll(docs []spec.StepDoc) { s.cli.RenderIndexAll(docs) }
func (s *streamSink) RenderIndexStep(doc spec.StepDoc)   { s.cli.RenderIndexStep(doc) }
func (s *streamSink) RenderLegend()                      { s.cli.RenderLegend() }
