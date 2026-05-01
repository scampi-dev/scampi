// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import "scampi.dev/scampi/diagnostic/event"

// Displayer is the sink for engine events. Implementations live in
// render/* and translate events into a presentation surface
// (CLI, JSON, …). The interface lives here, alongside event types,
// because it is the public consumption contract for diagnostic
// output — not because rendering is part of the diagnostic package.
type Displayer interface {
	EmitEngineLifecycle(e event.EngineEvent)
	EmitPlanLifecycle(e event.PlanEvent)
	EmitActionLifecycle(e event.ActionEvent)
	EmitOpLifecycle(e event.OpEvent)

	EmitIndexAll(e event.IndexAllEvent)
	EmitIndexStep(e event.IndexStepEvent)
	EmitInspect(e event.InspectEvent)

	EmitLegend()

	EmitEngineDiagnostic(e event.EngineDiagnostic)
	EmitPlanDiagnostic(e event.PlanDiagnostic)
	EmitActionDiagnostic(e event.ActionDiagnostic)
	EmitOpDiagnostic(e event.OpDiagnostic)

	Interrupt()
	Close()
}
