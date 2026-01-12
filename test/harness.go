package test

import (
	"encoding/json"
	"fmt"
	"testing"

	"godoit.dev/doit/diagnostic/event"
)

type ExpectedDiagnostics struct {
	Abort       bool                 `json:"abort"`
	Diagnostics []ExpectedDiagnostic `json:"diagnostics"`
}

type ExpectedDiagnostic struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Scope    string `json:"scope"`
	Severity string `json:"severity"`

	Source *ExpectedSource `json:"source,omitempty"`
	Unit   *ExpectedUnit   `json:"unit,omitempty"`
}

type ExpectedSource struct {
	Line int `json:"line"`
}

type ExpectedUnit struct {
	Index int    `json:"index"`
	Kind  string `json:"kind"`
}

type (
	events             []event.Event
	recordingDisplayer struct {
		events events
	}
)

func (r *recordingDisplayer) Emit(e event.Event) {
	r.events = append(r.events, e)
}

func (r *recordingDisplayer) Close() {}

func (r *recordingDisplayer) String() string {
	return r.events.String()
}

func (r *recordingDisplayer) dump(t *testing.T) {
	t.Helper()
	_, _ = fmt.Fprintln(t.Output(), r)
}

func (e events) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- DIAGNOSTICS -----\n" +
		string(j)
}
