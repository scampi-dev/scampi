// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"encoding/json"
	"fmt"
	"io"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/spec"
)

type (
	Events []event.Event

	// RecordingDisplayer captures every emitted event for test
	// inspection. It is an Output only: wrap it in diagnostic.NewEmitter
	// (the emitter serializes delivery, so this needs no lock) and keep the
	// reference to inspect Events afterward. Events lands in arrival order;
	// Diagnostics, Changes, and Results are populated alongside for
	// convenient typed assertions.
	RecordingDisplayer struct {
		Events      Events
		Diagnostics []event.Event // Error/Warning/Info entries
		Changes     []event.Change
		Results     []event.Result
	}
)

func (r *RecordingDisplayer) RenderEvent(e event.Event) {
	r.Events = append(r.Events, e)
	switch v := e.(type) {
	case event.Error, event.Warning, event.Info:
		r.Diagnostics = append(r.Diagnostics, v)
	case event.Change:
		r.Changes = append(r.Changes, v)
	case event.Result:
		r.Results = append(r.Results, v)
	}
}

// One-shot renders are no-ops: tests assert on the captured event stream, not
// on rendered command output. Present only to satisfy diagnostic.Output.

func (r *RecordingDisplayer) RenderSummary(result.Execution, bool) {}
func (r *RecordingDisplayer) RenderPlan(result.Plan)               {}
func (r *RecordingDisplayer) RenderInspect(result.Inspect)         {}
func (r *RecordingDisplayer) RenderIndexAll([]spec.StepDoc)        {}
func (r *RecordingDisplayer) RenderIndexStep(spec.StepDoc)         {}
func (r *RecordingDisplayer) RenderLegend()                        {}

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
		ids = append(ids, string(event.TemplateOf(e).ID))
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

func (e Events) String() string { return MarshalSection("EVENTS", e) }

// NoopEmitter returns an emitter that discards all events, for tests that run
// the pipeline but don't inspect output.
func NoopEmitter() *diagnostic.Emitter {
	return diagnostic.NewEmitter(diagnostic.Policy{}, diagnostic.Discard{})
}
