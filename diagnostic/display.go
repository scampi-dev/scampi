// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import "scampi.dev/scampi/diagnostic/event"

// Displayer is the sink for engine events. Implementations live in
// render/* and translate events into a presentation surface
// (CLI, JSON, …). The interface lives here, alongside event types,
// because it is the public consumption contract for diagnostic
// output — not because rendering is part of the diagnostic package.
type Displayer interface {
	EmitGraph(e event.GraphEvent)

	EmitLegend()

	EmitEngineDiagnostic(e event.EngineDiagnostic)
	EmitPlanDiagnostic(e event.PlanDiagnostic)
	EmitActionDiagnostic(e event.ActionDiagnostic)
	EmitOpDiagnostic(e event.OpDiagnostic)

	EmitDiagnostic(e event.Diagnostic)
	EmitChange(e event.Change)
	EmitProgress(e event.Progress)

	Interrupt()
	Close()
}
