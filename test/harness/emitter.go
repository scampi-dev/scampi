// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"scampi.dev/scampi/diagnostic/event"
)

type (
	EngineEvents       []event.EngineEvent
	PlanEvents         []event.PlanEvent
	ActionEvents       []event.ActionEvent
	OpEvents           []event.OpEvent
	IndexAllEvents     []event.IndexAllEvent
	EngineDiagnostics  []event.EngineDiagnostic
	PlanDiagnostics    []event.PlanDiagnostic
	ActionDiagnostics  []event.ActionDiagnostic
	OpDiagnostics      []event.OpDiagnostic
	IndexStepEvents    []event.IndexStepEvent
	RecordingDisplayer struct {
		mu                sync.Mutex
		EngineEvents      EngineEvents
		PlanEvents        PlanEvents
		ActionEvents      ActionEvents
		OpEvents          OpEvents
		EngineDiagnostics EngineDiagnostics
		PlanDiagnostics   PlanDiagnostics
		ActionDiagnostics ActionDiagnostics
		OpDiagnostics     OpDiagnostics
		IndexAllEvents    IndexAllEvents
		IndexStepEvents   IndexStepEvents
	}
	NoopEmitter struct{}
)

func (r *RecordingDisplayer) EmitEngineLifecycle(e event.EngineEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.EngineEvents = append(r.EngineEvents, e)
}

func (r *RecordingDisplayer) EmitPlanLifecycle(e event.PlanEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.PlanEvents = append(r.PlanEvents, e)
}

func (r *RecordingDisplayer) EmitActionLifecycle(e event.ActionEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ActionEvents = append(r.ActionEvents, e)
}

func (r *RecordingDisplayer) EmitOpLifecycle(e event.OpEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.OpEvents = append(r.OpEvents, e)
}

func (r *RecordingDisplayer) EmitEngineDiagnostic(e event.EngineDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.EngineDiagnostics = append(r.EngineDiagnostics, e)
}

func (r *RecordingDisplayer) EmitPlanDiagnostic(e event.PlanDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.PlanDiagnostics = append(r.PlanDiagnostics, e)
}

func (r *RecordingDisplayer) EmitActionDiagnostic(e event.ActionDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ActionDiagnostics = append(r.ActionDiagnostics, e)
}

func (r *RecordingDisplayer) EmitOpDiagnostic(e event.OpDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.OpDiagnostics = append(r.OpDiagnostics, e)
}

func (r *RecordingDisplayer) EmitIndexAll(e event.IndexAllEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.IndexAllEvents = append(r.IndexAllEvents, e)
}

func (r *RecordingDisplayer) EmitIndexStep(e event.IndexStepEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.IndexStepEvents = append(r.IndexStepEvents, e)
}

func (r *RecordingDisplayer) EmitInspect(_ event.InspectEvent) {}

func (r *RecordingDisplayer) EmitLegend() {}

func (r *RecordingDisplayer) Interrupt() {}

func (r *RecordingDisplayer) Close() {}

func (r *RecordingDisplayer) String() string {
	return r.EngineEvents.String() + "\n" +
		r.PlanEvents.String() + "\n" +
		r.ActionEvents.String() + "\n" +
		r.OpEvents.String() + "\n" +
		r.IndexAllEvents.String() + "\n" +
		r.IndexStepEvents.String() + "\n" +
		r.EngineDiagnostics.String() + "\n" +
		r.PlanDiagnostics.String() + "\n" +
		r.ActionDiagnostics.String() + "\n" +
		r.OpDiagnostics.String()
}

func (r *RecordingDisplayer) Dump(w io.Writer) {
	_, _ = fmt.Fprintln(w, r)
}

func (r *RecordingDisplayer) CountChangedOps() int {
	count := 0
	for _, ev := range r.OpEvents {
		if ev.ExecuteDetail != nil && ev.ExecuteDetail.Changed {
			count++
		}
	}
	return count
}

func (r *RecordingDisplayer) CollectDiagnosticIDs() []string {
	var ids []string
	for _, d := range r.EngineDiagnostics {
		ids = append(ids, string(d.Detail.Template.ID))
	}
	for _, d := range r.PlanDiagnostics {
		ids = append(ids, string(d.Detail.Template.ID))
	}
	for _, d := range r.ActionDiagnostics {
		ids = append(ids, string(d.Detail.Template.ID))
	}
	for _, d := range r.OpDiagnostics {
		ids = append(ids, string(d.Detail.Template.ID))
	}
	return ids
}

func MarshalSection(header string, v any) string {
	j, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- " + header + " -----\n" + string(j)
}

func (e EngineEvents) String() string      { return MarshalSection("ENGINE EVENTS", e) }
func (e PlanEvents) String() string        { return MarshalSection("PLAN EVENTS", e) }
func (e ActionEvents) String() string      { return MarshalSection("ACTION EVENTS", e) }
func (e OpEvents) String() string          { return MarshalSection("OP EVENTS", e) }
func (e IndexAllEvents) String() string    { return MarshalSection("INDEX_ALL EVENTS", e) }
func (e IndexStepEvents) String() string   { return MarshalSection("INDEX_STEP EVENTS", e) }
func (e EngineDiagnostics) String() string { return MarshalSection("ENGINE DIAGNOSTICS", e) }
func (e PlanDiagnostics) String() string   { return MarshalSection("PLAN DIAGNOSTICS", e) }
func (e ActionDiagnostics) String() string { return MarshalSection("ACTION DIAGNOSTICS", e) }
func (e OpDiagnostics) String() string     { return MarshalSection("OP DIAGNOSTICS", e) }

func (NoopEmitter) EmitEngineLifecycle(event.EngineEvent)       {}
func (NoopEmitter) EmitPlanLifecycle(event.PlanEvent)           {}
func (NoopEmitter) EmitActionLifecycle(event.ActionEvent)       {}
func (NoopEmitter) EmitOpLifecycle(event.OpEvent)               {}
func (NoopEmitter) EmitEngineDiagnostic(event.EngineDiagnostic) {}
func (NoopEmitter) EmitPlanDiagnostic(event.PlanDiagnostic)     {}
func (NoopEmitter) EmitActionDiagnostic(event.ActionDiagnostic) {}
func (NoopEmitter) EmitOpDiagnostic(event.OpDiagnostic)         {}
func (NoopEmitter) EmitIndexAll(event.IndexAllEvent)            {}
func (NoopEmitter) EmitIndexStep(event.IndexStepEvent)          {}
func (NoopEmitter) EmitInspect(event.InspectEvent)              {}
