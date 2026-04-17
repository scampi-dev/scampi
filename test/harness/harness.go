// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
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
	Step   *ExpectedStep   `json:"step,omitempty"`
}

type ExpectedSource struct {
	StartLine int `json:"start_line"`
	StartCol  int `json:"start_col"`
	EndLine   int `json:"end_line"`
	EndCol    int `json:"end_col"`
}

type ExpectedStep struct {
	Index int    `json:"index"`
	Kind  string `json:"kind"`
}

func AbsPath(p string) string {
	r, err := filepath.Abs(p)
	if err != nil {
		panic(err)
	}

	return r
}

func ReadDirOrDie(name string) []os.DirEntry {
	res, err := os.ReadDir(name)
	if err != nil {
		panic(err)
	}

	return res
}

func ReadDirSafe(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func ReadFileSafe(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func ReadOrDie(name string) []byte {
	data, err := os.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}

func WriteOrDie(name string, data []byte, perm os.FileMode) {
	if err := os.WriteFile(name, data, perm); err != nil {
		panic(err)
	}
}

// LoadExpected reads and unmarshals an expected diagnostics JSON file.
func LoadExpected(t *testing.T, path string) ExpectedDiagnostics {
	t.Helper()

	data := ReadOrDie(path)
	var e ExpectedDiagnostics
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

type collectedDiagnostic struct {
	scope    string
	severity string
	template event.Template
	step     *event.StepDetail
}

func collectDiagnostics(rec *RecordingDisplayer) []collectedDiagnostic {
	var collected []collectedDiagnostic

	for _, d := range rec.EngineDiagnostics {
		collected = append(collected, collectedDiagnostic{
			scope:    "ScopeEngine",
			severity: d.Severity.String(),
			template: d.Detail.Template,
			step:     nil,
		})
	}

	for _, d := range rec.PlanDiagnostics {
		step := d.Step
		collected = append(collected, collectedDiagnostic{
			scope:    "ScopePlan",
			severity: d.Severity.String(),
			template: d.Detail.Template,
			step:     &step,
		})
	}

	for _, d := range rec.ActionDiagnostics {
		step := d.Step
		collected = append(collected, collectedDiagnostic{
			scope:    "ScopeAction",
			severity: d.Severity.String(),
			template: d.Detail.Template,
			step:     &step,
		})
	}

	for _, d := range rec.OpDiagnostics {
		step := d.Step
		collected = append(collected, collectedDiagnostic{
			scope:    "ScopeOp",
			severity: d.Severity.String(),
			template: d.Detail.Template,
			step:     &step,
		})
	}

	return collected
}

// AssertDiagnostics compares recorded diagnostics against expected ones.
func AssertDiagnostics(
	t *testing.T,
	rec *RecordingDisplayer,
	expect []ExpectedDiagnostic,
	cfgPath string,
) {
	t.Helper()

	actual := collectDiagnostics(rec)

	if len(actual) != len(expect) {
		t.Fatalf("expected %d diagnostics, got %d", len(expect), len(actual))
	}

	for i, exp := range expect {
		got := actual[i]

		if got.severity != exp.Severity {
			t.Fatalf("[%d] expected severity %q, got %q", i, exp.Severity, got.severity)
		}

		if exp.Kind != "DiagnosticRaised" {
			t.Fatalf("[%d] unexpected kind in test data: %q (should always be DiagnosticRaised)", i, exp.Kind)
		}

		if got.scope != exp.Scope {
			t.Fatalf("[%d] expected scope %q, got %q", i, exp.Scope, got.scope)
		}

		tmpl := got.template

		if tmpl.ID != errs.Code(exp.ID) {
			t.Fatalf("[%d] expected id %q, got %q", i, exp.ID, tmpl.ID)
		}

		if exp.Source != nil {
			if tmpl.Source == nil {
				t.Fatalf("[%d] expected source, got nil", i)
			}
			if tmpl.Source.Filename != cfgPath {
				t.Fatalf("[%d] expected source file %q, got %q", i, cfgPath, tmpl.Source.Filename)
			}
			s := tmpl.Source
			e := exp.Source
			startMatch := s.StartLine == e.StartLine && s.StartCol == e.StartCol
			endMatch := s.EndLine == e.EndLine && s.EndCol == e.EndCol
			if !startMatch || !endMatch {
				t.Fatalf("[%d] source span mismatch:\n  got:  %d:%d → %d:%d\n  want: %d:%d → %d:%d",
					i,
					s.StartLine, s.StartCol, s.EndLine, s.EndCol,
					e.StartLine, e.StartCol, e.EndLine, e.EndCol,
				)
			}
		}

		if exp.Step != nil {
			if got.step == nil {
				t.Fatalf("[%d] expected step, got nil", i)
			}
			if got.step.StepIndex != exp.Step.Index || got.step.StepKind != exp.Step.Kind {
				t.Fatalf("[%d] step mismatch: got {%d, %q}, want {%d, %q}",
					i, got.step.StepIndex, got.step.StepKind, exp.Step.Index, exp.Step.Kind)
			}
		}
	}
}
