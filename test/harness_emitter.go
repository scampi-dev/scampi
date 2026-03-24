// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"scampi.dev/scampi/diagnostic/event"
)

type (
	engineEvents       []event.EngineEvent
	planEvents         []event.PlanEvent
	actionEvents       []event.ActionEvent
	opEvents           []event.OpEvent
	indexAllEvents     []event.IndexAllEvent
	engineDiagnostics  []event.EngineDiagnostic
	planDiagnostics    []event.PlanDiagnostic
	actionDiagnostics  []event.ActionDiagnostic
	opDiagnostics      []event.OpDiagnostic
	indexStepEvents    []event.IndexStepEvent
	recordingDisplayer struct {
		mu                sync.Mutex
		engineEvents      engineEvents
		planEvents        planEvents
		actionEvents      actionEvents
		opEvents          opEvents
		engineDiagnostics engineDiagnostics
		planDiagnostics   planDiagnostics
		actionDiagnostics actionDiagnostics
		opDiagnostics     opDiagnostics
		indexAllEvents    indexAllEvents
		indexStepEvents   indexStepEvents
	}
	noopEmitter struct{}
)

func (r *recordingDisplayer) EmitEngineLifecycle(e event.EngineEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engineEvents = append(r.engineEvents, e)
}

func (r *recordingDisplayer) EmitPlanLifecycle(e event.PlanEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.planEvents = append(r.planEvents, e)
}

func (r *recordingDisplayer) EmitActionLifecycle(e event.ActionEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actionEvents = append(r.actionEvents, e)
}

func (r *recordingDisplayer) EmitOpLifecycle(e event.OpEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opEvents = append(r.opEvents, e)
}

func (r *recordingDisplayer) EmitEngineDiagnostic(e event.EngineDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engineDiagnostics = append(r.engineDiagnostics, e)
}

func (r *recordingDisplayer) EmitPlanDiagnostic(e event.PlanDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.planDiagnostics = append(r.planDiagnostics, e)
}

func (r *recordingDisplayer) EmitActionDiagnostic(e event.ActionDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actionDiagnostics = append(r.actionDiagnostics, e)
}

func (r *recordingDisplayer) EmitOpDiagnostic(e event.OpDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opDiagnostics = append(r.opDiagnostics, e)
}

func (r *recordingDisplayer) EmitIndexAll(e event.IndexAllEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexAllEvents = append(r.indexAllEvents, e)
}

func (r *recordingDisplayer) EmitIndexStep(e event.IndexStepEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexStepEvents = append(r.indexStepEvents, e)
}

func (r *recordingDisplayer) EmitInspect(_ event.InspectEvent) {}

func (r *recordingDisplayer) EmitLegend() {}

func (r *recordingDisplayer) Interrupt() {}

func (r *recordingDisplayer) Close() {}

func (r *recordingDisplayer) String() string {
	return r.engineEvents.String() + "\n" +
		r.planEvents.String() + "\n" +
		r.actionEvents.String() + "\n" +
		r.opEvents.String() + "\n" +
		r.indexAllEvents.String() + "\n" +
		r.indexStepEvents.String() + "\n" +
		r.engineDiagnostics.String() + "\n" +
		r.planDiagnostics.String() + "\n" +
		r.actionDiagnostics.String() + "\n" +
		r.opDiagnostics.String()
}

func (r *recordingDisplayer) dump(w io.Writer) {
	_, _ = fmt.Fprintln(w, r)
}

func (r *recordingDisplayer) countChangedOps() int {
	count := 0
	for _, ev := range r.opEvents {
		if ev.ExecuteDetail != nil && ev.ExecuteDetail.Changed {
			count++
		}
	}
	return count
}

func (r *recordingDisplayer) collectDiagnosticIDs() []string {
	var ids []string
	for _, d := range r.engineDiagnostics {
		ids = append(ids, d.Detail.Template.ID)
	}
	for _, d := range r.planDiagnostics {
		ids = append(ids, d.Detail.Template.ID)
	}
	for _, d := range r.actionDiagnostics {
		ids = append(ids, d.Detail.Template.ID)
	}
	for _, d := range r.opDiagnostics {
		ids = append(ids, d.Detail.Template.ID)
	}
	return ids
}

func marshalSection(header string, v any) string {
	j, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- " + header + " -----\n" + string(j)
}

func (e engineEvents) String() string      { return marshalSection("ENGINE EVENTS", e) }
func (e planEvents) String() string        { return marshalSection("PLAN EVENTS", e) }
func (e actionEvents) String() string      { return marshalSection("ACTION EVENTS", e) }
func (e opEvents) String() string          { return marshalSection("OP EVENTS", e) }
func (e indexAllEvents) String() string    { return marshalSection("INDEX_ALL EVENTS", e) }
func (e indexStepEvents) String() string   { return marshalSection("INDEX_STEP EVENTS", e) }
func (e engineDiagnostics) String() string { return marshalSection("ENGINE DIAGNOSTICS", e) }
func (e planDiagnostics) String() string   { return marshalSection("PLAN DIAGNOSTICS", e) }
func (e actionDiagnostics) String() string { return marshalSection("ACTION DIAGNOSTICS", e) }
func (e opDiagnostics) String() string     { return marshalSection("OP DIAGNOSTICS", e) }

func (noopEmitter) EmitEngineLifecycle(event.EngineEvent)       {}
func (noopEmitter) EmitPlanLifecycle(event.PlanEvent)           {}
func (noopEmitter) EmitActionLifecycle(event.ActionEvent)       {}
func (noopEmitter) EmitOpLifecycle(event.OpEvent)               {}
func (noopEmitter) EmitEngineDiagnostic(event.EngineDiagnostic) {}
func (noopEmitter) EmitPlanDiagnostic(event.PlanDiagnostic)     {}
func (noopEmitter) EmitActionDiagnostic(event.ActionDiagnostic) {}
func (noopEmitter) EmitOpDiagnostic(event.OpDiagnostic)         {}
func (noopEmitter) EmitIndexAll(event.IndexAllEvent)            {}
func (noopEmitter) EmitIndexStep(event.IndexStepEvent)          {}
func (noopEmitter) EmitInspect(event.InspectEvent)              {}
