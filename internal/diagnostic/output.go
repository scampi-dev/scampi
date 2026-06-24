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
type Output interface {
	RenderEvent(event.Event)
	RenderSummary(rep result.Execution, checkOnly bool)
	RenderPlan(result.Plan)
	RenderInspect(result.Inspect)
	RenderIndexAll([]spec.StepDoc)
	RenderIndexStep(spec.StepDoc)
	RenderLegend()
}
