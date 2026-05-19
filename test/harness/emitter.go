// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
)

type (
	Events []event.Event

	// RecordingDisplayer captures every emitted event for test
	// inspection. Implements diagnostic.Displayer. Events lands in
	// arrival order; Diagnostics and Changes are populated alongside
	// for convenient typed assertions.
	RecordingDisplayer struct {
		mu          sync.Mutex
		Events      Events
		Diagnostics []event.Event // Error/Warning/Info entries
		Changes     []event.Change
	}
	// NoopEmitter discards every event. Implements diagnostic.Emitter.
	NoopEmitter struct{}
)

func (r *RecordingDisplayer) Raise(err diagnostic.Raisable) {
	r.Emit(err.Diagnostic())
}

func (r *RecordingDisplayer) Emit(e event.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Events = append(r.Events, e)
	switch v := e.(type) {
	case event.Error, event.Warning, event.Info:
		r.Diagnostics = append(r.Diagnostics, v)
	case event.Change:
		r.Changes = append(r.Changes, v)
	}
}

func (r *RecordingDisplayer) EmitLegend() {}
func (r *RecordingDisplayer) Interrupt()  {}
func (r *RecordingDisplayer) Close()      {}

func (r *RecordingDisplayer) String() string {
	return r.Events.String()
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
	for _, e := range r.Diagnostics {
		ids = append(ids, string(TemplateOf(e).ID))
	}
	return ids
}

// TemplateOf extracts the Template from any diagnostic-shaped event.
func TemplateOf(ev event.Event) event.Template {
	switch v := ev.(type) {
	case event.Error:
		return v.Template
	case event.Warning:
		return v.Template
	case event.Info:
		return v.Template
	default:
		return event.Template{}
	}
}

func MarshalSection(header string, v any) string {
	j, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- " + header + " -----\n" + string(j)
}

func (e Events) String() string { return MarshalSection("EVENTS", e) }

func (NoopEmitter) Emit(event.Event)          {}
func (NoopEmitter) Raise(diagnostic.Raisable) {}
