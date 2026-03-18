// SPDX-License-Identifier: GPL-3.0-only

package render

import "scampi.dev/scampi/diagnostic/event"

// Displayer defines the interface for rendering engine events.
type Displayer interface {
	EmitEngineLifecycle(e event.EngineEvent)
	EmitPlanLifecycle(e event.PlanEvent)
	EmitActionLifecycle(e event.ActionEvent)
	EmitOpLifecycle(e event.OpEvent)

	EmitIndexAll(e event.IndexAllEvent)
	EmitIndexStep(e event.IndexStepEvent)

	EmitLegend()

	EmitEngineDiagnostic(e event.EngineDiagnostic)
	EmitPlanDiagnostic(e event.PlanDiagnostic)
	EmitActionDiagnostic(e event.ActionDiagnostic)
	EmitOpDiagnostic(e event.OpDiagnostic)

	Interrupt()
	Close()
}
