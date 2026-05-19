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
	EngineDiagnostics  []event.EngineDiagnostic
	PlanDiagnostics    []event.PlanDiagnostic
	ActionDiagnostics  []event.ActionDiagnostic
	OpDiagnostics      []event.OpDiagnostic
	Diagnostics        []event.Diagnostic
	Changes            []event.Change
	ProgressEvents     []event.Progress
	RecordingDisplayer struct {
		mu                sync.Mutex
		EngineDiagnostics EngineDiagnostics
		PlanDiagnostics   PlanDiagnostics
		ActionDiagnostics ActionDiagnostics
		OpDiagnostics     OpDiagnostics
		Diagnostics       Diagnostics
		Changes           Changes
		ProgressEvents    ProgressEvents
	}
	NoopEmitter struct{}
)

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

func (r *RecordingDisplayer) EmitInspect(_ event.InspectEvent) {}

func (r *RecordingDisplayer) EmitGraph(_ event.GraphEvent)     {}
func (r *RecordingDisplayer) EmitPlanOutput(_ event.PlanEvent) {}

func (r *RecordingDisplayer) EmitDiagnostic(e event.Diagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Diagnostics = append(r.Diagnostics, e)
}

func (r *RecordingDisplayer) EmitChange(e event.Change) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Changes = append(r.Changes, e)
}

func (r *RecordingDisplayer) EmitProgress(e event.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ProgressEvents = append(r.ProgressEvents, e)
}

func (r *RecordingDisplayer) EmitLegend() {}

func (r *RecordingDisplayer) Interrupt() {}

func (r *RecordingDisplayer) Close() {}

func (r *RecordingDisplayer) String() string {
	return r.EngineDiagnostics.String() + "\n" +
		r.PlanDiagnostics.String() + "\n" +
		r.ActionDiagnostics.String() + "\n" +
		r.OpDiagnostics.String() + "\n" +
		r.Diagnostics.String() + "\n" +
		r.Changes.String() + "\n" +
		r.ProgressEvents.String()
}

func (r *RecordingDisplayer) Dump(w io.Writer) {
	_, _ = fmt.Fprintln(w, r)
}

func (r *RecordingDisplayer) CountChangedOps() int {
	count := 0
	for _, c := range r.Changes {
		if c.Phase == event.ChangeExecuted {
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

func (e EngineDiagnostics) String() string { return MarshalSection("ENGINE DIAGNOSTICS", e) }
func (e PlanDiagnostics) String() string   { return MarshalSection("PLAN DIAGNOSTICS", e) }
func (e ActionDiagnostics) String() string { return MarshalSection("ACTION DIAGNOSTICS", e) }
func (e OpDiagnostics) String() string     { return MarshalSection("OP DIAGNOSTICS", e) }
func (e Diagnostics) String() string       { return MarshalSection("DIAGNOSTICS", e) }
func (e Changes) String() string           { return MarshalSection("CHANGES", e) }
func (e ProgressEvents) String() string    { return MarshalSection("PROGRESS", e) }

func (NoopEmitter) EmitEngineDiagnostic(event.EngineDiagnostic) {}
func (NoopEmitter) EmitPlanDiagnostic(event.PlanDiagnostic)     {}
func (NoopEmitter) EmitActionDiagnostic(event.ActionDiagnostic) {}
func (NoopEmitter) EmitOpDiagnostic(event.OpDiagnostic)         {}
func (NoopEmitter) EmitGraph(event.GraphEvent)                  {}
func (NoopEmitter) EmitPlanOutput(event.PlanEvent)              {}
func (NoopEmitter) EmitDiagnostic(event.Diagnostic)             {}
func (NoopEmitter) EmitChange(event.Change)                     {}
func (NoopEmitter) EmitProgress(event.Progress)                 {}
