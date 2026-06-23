// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"testing"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/target"
)

// Registry dedup
// -----------------------------------------------------------------------------

func TestAddMemTarget_DedupesByName(t *testing.T) {
	reg := NewTestRegistry()
	entry1 := reg.AddMemTarget(MemTargetEntry{
		Name: "m",
		Mock: target.NewMemTarget(),
	})
	entry2 := reg.AddMemTarget(MemTargetEntry{
		Name: "m",
		Mock: target.NewMemTarget(),
	})
	if entry1.Mock != entry2.Mock {
		t.Errorf("second AddMemTarget should return the first mock")
	}
	if len(reg.MemTargets()) != 1 {
		t.Errorf("expected 1 entry, got %d", len(reg.MemTargets()))
	}
}

// Seed functions with wrong-typed inputs — should not panic
// -----------------------------------------------------------------------------

func TestSeedMemTarget_WrongTypes(t *testing.T) {
	mock := target.NewMemTarget()

	// All fields are wrong types — should be silently skipped.
	initial := &eval.StructVal{
		TypeName: "InitialState",
		RetType:  "InitialState",
		Fields: map[string]eval.Value{
			"files":    &eval.IntVal{V: 42},              // want MapVal
			"packages": &eval.StringVal{V: "not-a-list"}, // want ListVal
			"services": &eval.BoolVal{V: true},           // want MapVal
			"dirs":     &eval.IntVal{V: 0},               // want ListVal
			"symlinks": &eval.StringVal{V: "nope"},       // want MapVal
		},
	}
	seedMemTarget(mock, initial)

	// Nothing should have been seeded — no panic.
	if len(mock.Files) != 0 {
		t.Errorf("files: %d", len(mock.Files))
	}
	if len(mock.Pkgs) != 0 {
		t.Errorf("pkgs: %d", len(mock.Pkgs))
	}
}

func TestSeedMemTarget_NilInitial(t *testing.T) {
	mock := target.NewMemTarget()
	seedMemTarget(mock, nil)
	if len(mock.Files) != 0 {
		t.Errorf("expected empty files")
	}
}

func TestExtractInlineContent_Nil(t *testing.T) {
	if got := extractInlineContent(nil); got != "" {
		t.Errorf("nil: got %q", got)
	}
	sv := &eval.StructVal{Fields: map[string]eval.Value{}}
	if got := extractInlineContent(sv); got != "" {
		t.Errorf("no content field: got %q", got)
	}
}

func TestTestSetupError(t *testing.T) {
	e := &TestSetupError{Reason: "wrong config"}
	if !contains(e.Error(), "wrong config") {
		t.Errorf("Error() = %q", e.Error())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
