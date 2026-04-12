// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"context"
	"testing"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// initialState builds an `initial` value tree the way the linker
// would hand it to MemTargetType.Create — a *eval.StructVal with
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

func TestMemTargetType_SeedsAllSlots(t *testing.T) {
	reg := NewTestRegistry()
	tt := MemTargetType{Registry: reg}

	cfg := &MemTargetConfig{
		Name:    "mock",
		Initial: initialState(t),
	}
	got, err := tt.Create(context.Background(), nil, spec.TargetInstance{Config: cfg})
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

func TestMemTargetType_RegistersInRegistry(t *testing.T) {
	reg := NewTestRegistry()
	tt := MemTargetType{Registry: reg}

	expect := &eval.StructVal{
		TypeName: "ExpectedState",
		RetType:  "ExpectedState",
		Fields:   map[string]eval.Value{},
	}
	cfg := &MemTargetConfig{Name: "mock", Expect: expect}

	if _, err := tt.Create(context.Background(), nil, spec.TargetInstance{Config: cfg}); err != nil {
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

func TestMemTargetType_NilInitialAndExpect(t *testing.T) {
	reg := NewTestRegistry()
	tt := MemTargetType{Registry: reg}
	cfg := &MemTargetConfig{Name: "mock"}

	got, err := tt.Create(context.Background(), nil, spec.TargetInstance{Config: cfg})
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

func TestMemTargetType_NilRegistry(t *testing.T) {
	// Without a registry the constructor still works — the mock is
	// returned but not tracked. Useful for one-off Go-side tests.
	tt := MemTargetType{Registry: nil}
	got, err := tt.Create(context.Background(), nil, spec.TargetInstance{
		Config: &MemTargetConfig{Name: "anon"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := got.(*target.MemTarget); !ok {
		t.Fatalf("got %T", got)
	}
}

func TestMemRESTTargetType_RoutesAndRegistry(t *testing.T) {
	reg := NewTestRegistry()
	tt := MemRESTTargetType{Registry: reg}

	routes := &eval.MapVal{}
	routes.Keys = append(routes.Keys, &eval.StringVal{V: "POST /v1/sites"})
	routes.Values = append(routes.Values, &eval.StructVal{
		TypeName: "response",
		QualName: "test.response",
		RetType:  "Response",
		Fields: map[string]eval.Value{
			"status": &eval.IntVal{V: 201},
			"body":   &eval.StringVal{V: `{"id":1}`},
		},
	})

	expectReqs := &eval.ListVal{
		Items: []eval.Value{
			&eval.StructVal{
				TypeName: "request",
				QualName: "test.request",
				RetType:  "RequestMatcher",
				Fields: map[string]eval.Value{
					"method": &eval.StringVal{V: "POST"},
					"path":   &eval.StringVal{V: "/v1/sites"},
				},
			},
		},
	}

	cfg := &MemRESTTargetConfig{
		Name:           "api",
		BaseURL:        "http://localhost:8080",
		Routes:         routes,
		ExpectRequests: expectReqs,
	}

	got, err := tt.Create(context.Background(), nil, spec.TargetInstance{Config: cfg})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mock, ok := got.(*target.MemREST)
	if !ok {
		t.Fatalf("Create returned %T, want *target.MemREST", got)
	}
	resp, err := mock.Do(context.Background(), target.HTTPRequest{
		Method: "POST",
		Path:   "/v1/sites",
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if string(resp.Body) != `{"id":1}` {
		t.Errorf("body = %q", resp.Body)
	}

	entries := reg.MemRESTs()
	if len(entries) != 1 {
		t.Fatalf("got %d REST entries, want 1", len(entries))
	}
	if entries[0].Name != "api" {
		t.Errorf("name = %q", entries[0].Name)
	}
	if entries[0].ExpectRequests != expectReqs {
		t.Errorf("expect_requests not propagated")
	}
}

func TestMemTargetType_VerifyRoundTrip(t *testing.T) {
	// End-to-end Phase 2 + Phase 3: build a target via the
	// TargetType, mutate it as if engine.Apply ran ops, then run
	// VerifyMemTarget against the registry's stored expect.
	reg := NewTestRegistry()
	tt := MemTargetType{Registry: reg}

	expect := expectState(map[string]map[string]*eval.StructVal{
		"files": {
			"/etc/app.conf": matcher("has_substring", map[string]string{"substring": "listen 80"}),
		},
		"services": {
			"nginx": matcher("has_svc_status", map[string]string{"status": "running"}),
		},
	})

	cfg := &MemTargetConfig{Name: "mock", Expect: expect}
	got, _ := tt.Create(context.Background(), nil, spec.TargetInstance{Config: cfg})
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
