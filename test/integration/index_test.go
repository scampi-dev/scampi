// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"errors"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/test/harness"
)

func TestIndexAll_ReturnsWellFormedCatalog(t *testing.T) {
	docs := engine.IndexAll(context.Background())

	if len(docs) == 0 {
		t.Fatal("IndexAll returned no docs")
	}

	// Verify known steps are present with summaries.
	found := make(map[string]string)
	for _, d := range docs {
		found[d.Kind] = d.Summary
	}

	for _, kind := range []string{"copy", "symlink", "unarchive"} {
		desc, ok := found[kind]
		if !ok {
			t.Errorf("missing step %q in index", kind)
			continue
		}
		if desc == "" {
			t.Errorf("step %q has empty description", kind)
		}
	}
}

func TestIndexStep_EmitsWellFormedEvent(t *testing.T) {
	tests := []struct {
		kind           string
		wantSummary    string
		wantFields     []string
		wantFieldCount int
	}{
		{
			kind:           "copy",
			wantSummary:    "Copy files with owner and permission management",
			wantFields:     []string{"src", "dest", "perm", "owner", "group", "verify", "backup"},
			wantFieldCount: 10, // includes desc, promises, inputs
		},
		{
			kind:           "symlink",
			wantSummary:    "Create and manage symbolic links",
			wantFields:     []string{"target", "link"},
			wantFieldCount: 5, // includes desc, promises, inputs
		},
		{
			kind:           "pkg",
			wantSummary:    "Ensure packages are present, absent, or at the latest version on the target",
			wantFields:     []string{"packages", "state", "source"},
			wantFieldCount: 6, // includes desc, promises, inputs
		},
		{
			kind:           "firewall",
			wantSummary:    "Manage firewall rules via UFW or firewalld",
			wantFields:     []string{"port", "endport", "proto", "action"},
			wantFieldCount: 7, // includes desc, promises, inputs
		},
		{
			kind:           "sysctl",
			wantSummary:    "Manage kernel parameters via sysctl with optional persistence",
			wantFields:     []string{"key", "value"},
			wantFieldCount: 6, // includes desc, persist, promises, inputs
		},
		{
			kind:           "unarchive",
			wantSummary:    "Extract an archive to a target directory with optional recursive unpacking",
			wantFields:     []string{"src", "dest", "depth", "owner", "group", "perm"},
			wantFieldCount: 9, // includes desc, promises, inputs
		},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

			doc, err := engine.IndexStep(diagnostic.NewCtx(t.Context(), em), tt.kind)
			if err != nil {
				t.Fatalf("IndexStep(%q) failed: %v", tt.kind, err)
			}

			if doc.Kind != tt.kind {
				t.Errorf("Kind = %q, want %q", doc.Kind, tt.kind)
			}

			if doc.Summary != tt.wantSummary {
				t.Errorf("Summary = %q, want %q", doc.Summary, tt.wantSummary)
			}

			if len(doc.Fields) != tt.wantFieldCount {
				t.Errorf("len(Fields) = %d, want %d", len(doc.Fields), tt.wantFieldCount)
			}

			fieldNames := make(map[string]bool)
			for _, f := range doc.Fields {
				fieldNames[f.Name] = true

				if f.Type == "" {
					t.Errorf("field %q has empty Type", f.Name)
				}
			}

			for _, wantField := range tt.wantFields {
				if !fieldNames[wantField] {
					t.Errorf("missing field %q", wantField)
				}
			}
		})
	}
}

func TestIndexStep_UnknownKind_Aborts(t *testing.T) {
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	_, err := engine.IndexStep(diagnostic.NewCtx(t.Context(), em), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}

	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Errorf("expected AbortError, got %T", err)
	}
}

func TestIndexStep_FieldsHaveDocumentation(t *testing.T) {
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	doc, _ := engine.IndexStep(diagnostic.NewCtx(t.Context(), em), "symlink")

	for _, f := range doc.Fields {
		if f.Desc == "" {
			t.Errorf("field %q has no description", f.Name)
		}
	}
}

func TestIndexStep_DefaultsPopulated(t *testing.T) {
	tests := []struct {
		kind    string
		field   string
		wantDef string
	}{
		{"pkg", "state", `"present"`},
		{"service", "state", `"running"`},
		{"service", "enabled", `"true"`},
		{"firewall", "proto", `"tcp"`},
		{"firewall", "action", `"allow"`},
		{"sysctl", "persist", `"true"`},
		{"unarchive", "depth", `"0"`},
		{"user", "state", `"present"`},
	}

	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.field, func(t *testing.T) {
			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

			doc, err := engine.IndexStep(diagnostic.NewCtx(t.Context(), em), tt.kind)
			if err != nil {
				t.Fatalf("IndexStep(%q) failed: %v", tt.kind, err)
			}

			var found bool
			for _, f := range doc.Fields {
				if f.Name == tt.field {
					found = true
					if f.Default != tt.wantDef {
						t.Errorf("field %q default = %q, want %q", tt.field, f.Default, tt.wantDef)
					}
					break
				}
			}
			if !found {
				t.Errorf("field %q not found in %q step", tt.field, tt.kind)
			}
		})
	}
}

func TestIndexStep_RequiredFieldsMarkedCorrectly(t *testing.T) {
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	doc, _ := engine.IndexStep(diagnostic.NewCtx(t.Context(), em), "symlink")

	if doc.Kind == "" {
		t.Fatal("IndexStep returned empty doc")
	}

	fields := make(map[string]bool)
	for _, f := range doc.Fields {
		fields[f.Name] = f.Required
	}

	// target and link are required, desc is optional
	if !fields["target"] {
		t.Error("target should be required")
	}
	if !fields["link"] {
		t.Error("link should be required")
	}
	if fields["desc"] {
		t.Error("desc should be optional")
	}
}

func TestIndexStep_ExclusiveFieldsPopulated(t *testing.T) {
	tests := []struct {
		kind  string
		group string
		want  []string
	}{
		{"run", "trigger", []string{"check", "always"}},
	}

	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.group, func(t *testing.T) {
			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

			doc, err := engine.IndexStep(diagnostic.NewCtx(t.Context(), em), tt.kind)
			if err != nil {
				t.Fatalf("IndexStep(%q) failed: %v", tt.kind, err)
			}

			var got []string
			for _, f := range doc.Fields {
				if f.Exclusive == tt.group {
					got = append(got, f.Name)
				}
			}

			if len(got) != len(tt.want) {
				t.Fatalf("exclusive group %q: got %v, want %v", tt.group, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("exclusive group %q[%d] = %q, want %q", tt.group, i, got[i], tt.want[i])
				}
			}
		})
	}
}
