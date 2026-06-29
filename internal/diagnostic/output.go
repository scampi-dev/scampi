// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/spec"
)

// Output is a command-output backend. A single implementation renders both
// surfaces scampi has: the live event stream (RenderEvent, called per event as
// a run unfolds) and the one-shot command results (the rest, each called once
// with a value the engine returned). The CLI is one such backend; a future
// --json backend is another, implementing the same contract end to end.
//
// Every method is Render<thing> - the producer side (Emitter) emits, the sink
// side (Output) renders. The interface lives here, alongside the events and
// result types it consumes, because it is the public output contract, not
// because rendering belongs to the diagnostic package.
//
// Implementations are NOT required to be safe for concurrent use. The engine
// emits from parallel goroutines, but the Emitter serializes delivery, so every
// method is invoked one call at a time. Keep implementations lock-free; adding
// a per-Output mutex would double-lock and misstate the contract.
type Output interface {
	RenderEvent(event.Event)
	RenderSummary(rep result.Execution, checkOnly bool)
	RenderPlan(result.Plan)
	RenderInspect(result.Inspect)
	RenderIndexAll([]spec.StepDoc)
	RenderIndexStep(spec.StepDoc)
	RenderLegend()
}

// Discard is an Output that drops everything: a silent backend. It also serves
// as an embeddable base for test Outputs that only care about one method
// (embed Discard, override that method).
type Discard struct{}

func (Discard) RenderEvent(event.Event)              {}
func (Discard) RenderSummary(result.Execution, bool) {}
func (Discard) RenderPlan(result.Plan)               {}
func (Discard) RenderInspect(result.Inspect)         {}
func (Discard) RenderIndexAll([]spec.StepDoc)        {}
func (Discard) RenderIndexStep(spec.StepDoc)         {}
func (Discard) RenderLegend()                        {}
