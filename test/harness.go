package test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

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

func (r *recordingDisplayer) dump(w io.Writer) {
	_, _ = fmt.Fprintln(w, r)
}

func (e events) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- DIAGNOSTICS -----\n" +
		string(j)
}

func absPath(p string) string {
	r, err := filepath.Abs(p)
	if err != nil {
		panic(err)
	}

	return r
}

func currentUsr() *user.User {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	return usr
}

func readDirOrDie(name string) []os.DirEntry {
	res, err := os.ReadDir(name)
	if err != nil {
		panic(err)
	}

	return res
}

func readOrDie(name string) []byte {
	data, err := os.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}

func writeOrDie(name string, data []byte, perm os.FileMode) {
	if err := os.WriteFile(name, data, perm); err != nil {
		panic(err)
	}
}
