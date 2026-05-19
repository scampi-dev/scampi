// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
)

type (
	Diagnostics    []event.Diagnostic
	Changes        []event.Change
	ProgressEvents []event.Progress
	Events         []event.Event

	// RecordingDisplayer captures every streaming event for test
	// inspection. Implements diagnostic.Displayer.
	RecordingDisplayer struct {
		mu             sync.Mutex
		Diagnostics    Diagnostics
		Changes        Changes
		ProgressEvents ProgressEvents
		// Events records every event passed to Emit() in arrival order.
		// Tests can read it directly to assert against the sealed union,
		// or use the per-kind slices for type-narrowed assertions.
		Events Events
	}
	// NoopEmitter discards every event. Implements diagnostic.Emitter.
	NoopEmitter struct{}
)

func (r *RecordingDisplayer) Emit(e event.Event) {
	r.mu.Lock()
	r.Events = append(r.Events, e)
	r.mu.Unlock()
	switch v := e.(type) {
	case event.Error:
		r.EmitDiagnostic(event.Diagnostic{
			Time: v.Time, Severity: signal.Error, Template: v.Template, Cause: v.Cause,
		})
	case event.Warning:
		r.EmitDiagnostic(event.Diagnostic{
			Time: v.Time, Severity: signal.Warning, Template: v.Template, Cause: v.Cause,
		})
	case event.Info:
		r.EmitDiagnostic(event.Diagnostic{
			Time: v.Time, Severity: signal.Info, Template: v.Template, Cause: v.Cause,
		})
	case event.Change:
		r.EmitChange(v)
	case event.Progress:
		r.EmitProgress(v)
	}
}

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
func (r *RecordingDisplayer) Interrupt()  {}
func (r *RecordingDisplayer) Close()      {}

func (r *RecordingDisplayer) String() string {
	return r.Diagnostics.String() + "\n" +
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
	for _, d := range r.Diagnostics {
		ids = append(ids, string(d.Template.ID))
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

func (e Diagnostics) String() string    { return MarshalSection("DIAGNOSTICS", e) }
func (e Changes) String() string        { return MarshalSection("CHANGES", e) }
func (e ProgressEvents) String() string { return MarshalSection("PROGRESS", e) }

func (NoopEmitter) Emit(event.Event)                {}
func (NoopEmitter) EmitDiagnostic(event.Diagnostic) {}
func (NoopEmitter) EmitChange(event.Change)         {}
func (NoopEmitter) EmitProgress(event.Progress)     {}
