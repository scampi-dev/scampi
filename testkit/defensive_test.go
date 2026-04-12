// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"context"
	"testing"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/spec"
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

func TestAddMemREST_DedupesByName(t *testing.T) {
	reg := NewTestRegistry()
	entry1 := reg.AddMemREST(MemRESTEntry{
		Name: "api",
		Mock: target.NewMemREST(nil),
	})
	entry2 := reg.AddMemREST(MemRESTEntry{
		Name: "api",
		Mock: target.NewMemREST(nil), // different pointer
	})
	if entry1.Mock != entry2.Mock {
		t.Errorf("second AddMemREST should return the first mock, got different pointers")
	}
	if len(reg.MemRESTs()) != 1 {
		t.Errorf("expected 1 entry, got %d", len(reg.MemRESTs()))
	}
}

// buildResponse with headers
// -----------------------------------------------------------------------------

func TestMemRESTTargetType_ResponseHeaders(t *testing.T) {
	reg := NewTestRegistry()
	tt := MemRESTTargetType{Registry: reg}

	headers := &eval.MapVal{}
	headers.Keys = append(headers.Keys, &eval.StringVal{V: "X-Custom"})
	headers.Values = append(headers.Values, &eval.StringVal{V: "test-val"})

	routes := &eval.MapVal{}
	routes.Keys = append(routes.Keys, &eval.StringVal{V: "GET /x"})
	routes.Values = append(routes.Values, &eval.StructVal{
		TypeName: "response",
		RetType:  "Response",
		Fields: map[string]eval.Value{
			"status":  &eval.IntVal{V: 200},
			"body":    &eval.StringVal{V: "ok"},
			"headers": headers,
		},
	})

	cfg := &MemRESTTargetConfig{Name: "api", Routes: routes}
	got, err := tt.Create(context.Background(), nil, spec.TargetInstance{Config: cfg})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mock := got.(*target.MemREST)
	resp, _ := mock.Do(context.Background(), target.HTTPRequest{
		Method: "GET",
		Path:   "/x",
	})
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if h := resp.Headers["X-Custom"]; len(h) != 1 || h[0] != "test-val" {
		t.Errorf("headers = %v", resp.Headers)
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
			"files":    &eval.IntVal{V: 42},             // want MapVal
			"packages": &eval.StringVal{V: "not-a-list"}, // want ListVal
			"services": &eval.BoolVal{V: true},           // want MapVal
			"dirs":     &eval.IntVal{V: 0},               // want ListVal
			"symlinks": &eval.StringVal{V: "nope"},        // want MapVal
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
