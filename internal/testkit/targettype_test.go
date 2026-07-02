// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"testing"

	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

// initialState builds an `initial` value tree the way the linker
// would hand it to MemTargetKind.Create - a *eval.StructVal with
// per-slot map / list fields.
func initialState(t *testing.T) *eval.StructVal {
	t.Helper()
	sv := &eval.StructVal{
		TypeName: "InitialState",
		QualName: "test.InitialState",
		RetType:  "InitialState",
		Fields:   make(map[string]eval.Value),
	}
	// files = { "/etc/foo": posix.source_inline { content = "old" } }
	files := &eval.MapVal{}
	files.Keys = append(files.Keys, &eval.StringVal{V: "/etc/foo"})
	files.Values = append(files.Values, &eval.StructVal{
		TypeName: "source_inline",
		QualName: "posix.source_inline",
		RetType:  "Source",
		Fields:   map[string]eval.Value{"content": &eval.StringVal{V: "old"}},
	})
	sv.Fields["files"] = files
	// packages = ["nginx"]
	sv.Fields["packages"] = &eval.ListVal{
		Items: []eval.Value{&eval.StringVal{V: "nginx"}},
	}
	// services = { "nginx": "stopped" }
	svcs := &eval.MapVal{}
	svcs.Keys = append(svcs.Keys, &eval.StringVal{V: "nginx"})
	svcs.Values = append(svcs.Values, &eval.StringVal{V: "stopped"})
	sv.Fields["services"] = svcs
	// dirs = ["/var/log/myapp"]
	sv.Fields["dirs"] = &eval.ListVal{
		Items: []eval.Value{&eval.StringVal{V: "/var/log/myapp"}},
	}
	// symlinks = { "/usr/local/bin/foo": "/opt/foo/bin/foo" }
	syms := &eval.MapVal{}
	syms.Keys = append(syms.Keys, &eval.StringVal{V: "/usr/local/bin/foo"})
	syms.Values = append(syms.Values, &eval.StringVal{V: "/opt/foo/bin/foo"})
	sv.Fields["symlinks"] = syms
	return sv
}

func TestMemTargetKind_SeedsAllSlots(t *testing.T) {
	reg := NewTestRegistry()
	tt := MemTargetKind{Registry: reg}

	cfg := &MemTargetConfig{
		Name:    "mock",
		Initial: initialState(t),
	}
	got, err := tt.Create(t.Context(), nil, spec.DeclaredTarget{Config: cfg})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mock, ok := got.(*target.MemTarget)
	if !ok {
		t.Fatalf("Create returned %T, want *target.MemTarget", got)
	}

	if string(mock.Files["/etc/foo"]) != "old" {
		t.Errorf("file seed: got %q", mock.Files["/etc/foo"])
	}
	if !mock.Pkgs["nginx"] {
		t.Errorf("package seed: nginx missing")
	}
	if mock.Services["nginx"] {
		t.Errorf("service seed: nginx should be stopped, got running")
	}
	if _, ok := mock.Dirs["/var/log/myapp"]; !ok {
		t.Errorf("dir seed: /var/log/myapp missing")
	}
	if mock.Symlinks["/usr/local/bin/foo"] != "/opt/foo/bin/foo" {
		t.Errorf("symlink seed: got %q", mock.Symlinks["/usr/local/bin/foo"])
	}
}

func TestMemTargetKind_RegistersInRegistry(t *testing.T) {
	reg := NewTestRegistry()
	tt := MemTargetKind{Registry: reg}

	expect := &eval.StructVal{
		TypeName: "ExpectedState",
		RetType:  "ExpectedState",
		Fields:   map[string]eval.Value{},
	}
	cfg := &MemTargetConfig{Name: "mock", Expect: expect}

	if _, err := tt.Create(t.Context(), nil, spec.DeclaredTarget{Config: cfg}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	entries := reg.MemTargets()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Name != "mock" {
		t.Errorf("name = %q", entries[0].Name)
	}
	if entries[0].Mock == nil {
		t.Errorf("mock is nil")
	}
	if entries[0].Expect != expect {
		t.Errorf("expect not propagated")
	}
}

func TestMemTargetKind_NilInitialAndExpect(t *testing.T) {
	reg := NewTestRegistry()
	tt := MemTargetKind{Registry: reg}
	cfg := &MemTargetConfig{Name: "mock"}

	got, err := tt.Create(t.Context(), nil, spec.DeclaredTarget{Config: cfg})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mock := got.(*target.MemTarget)
	if len(mock.Files) != 0 || len(mock.Pkgs) != 0 {
		t.Errorf("expected empty mock, got files=%d pkgs=%d", len(mock.Files), len(mock.Pkgs))
	}
	entries := reg.MemTargets()
	if len(entries) != 1 || entries[0].Expect != nil {
		t.Errorf("registry: %+v", entries)
	}
}

func TestMemTargetKind_NilRegistry(t *testing.T) {
	// Without a registry the constructor still works - the mock is
	// returned but not tracked. Useful for one-off Go-side tests.
	tt := MemTargetKind{Registry: nil}
	got, err := tt.Create(t.Context(), nil, spec.DeclaredTarget{
		Config: &MemTargetConfig{Name: "anon"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := got.(*target.MemTarget); !ok {
		t.Fatalf("got %T", got)
	}
}

func TestMemTargetKind_VerifyRoundTrip(t *testing.T) {
	// End-to-end Phase 2 + Phase 3: build a target via the
	// TargetKind, mutate it as if engine.Apply ran ops, then run
	// VerifyMemTarget against the registry's stored expect.
	reg := NewTestRegistry()
	tt := MemTargetKind{Registry: reg}

	expect := expectState(map[string]map[string]*eval.StructVal{
		"files": {
			"/etc/app.conf": matcher("has_substring", map[string]string{"substring": "listen 80"}),
		},
		"services": {
			"nginx": matcher("has_svc_status", map[string]string{"status": "running"}),
		},
	})

	cfg := &MemTargetConfig{Name: "mock", Expect: expect}
	got, _ := tt.Create(t.Context(), nil, spec.DeclaredTarget{Config: cfg})
	mock := got.(*target.MemTarget)

	// Simulate engine.Apply: write a file, start a service.
	mock.Files["/etc/app.conf"] = []byte("server_name example.com\nlisten 80\n")
	mock.Services["nginx"] = true

	entries := reg.MemTargets()
	if len(entries) != 1 {
		t.Fatalf("expected 1 registry entry, got %d", len(entries))
	}
	mismatches := VerifyMemTarget(entries[0].Expect, entries[0].Mock)
	if len(mismatches) != 0 {
		t.Errorf("expected clean verify, got: %+v", mismatches)
	}
}

// Seed functions with wrong-typed inputs - should not panic
// -----------------------------------------------------------------------------

func TestSeedMemTarget_WrongTypes(t *testing.T) {
	mock := target.NewMemTarget()

	// All fields are wrong types - should be silently skipped.
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

	// Nothing should have been seeded - no panic.
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
