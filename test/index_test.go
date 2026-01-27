package test

import (
	"context"
	"errors"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
)

func TestIndexAll_EmitsWellFormedEvent(t *testing.T) {
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	err := engine.IndexAll(context.Background(), em)
	if err != nil {
		t.Fatalf("IndexAll failed: %v", err)
	}

	if len(rec.indexAllEvents) == 0 {
		t.Fatal("no IndexAllEvent emitted")
	}

	if len(rec.indexAllEvents) != 1 {
		t.Fatalf("too many IndexAllEvent emitted - have %d, want 1", len(rec.indexAllEvents))
	}

	e := rec.indexAllEvents[0]
	if len(e.Steps) == 0 {
		t.Fatal("IndexAllEvent has no steps")
	}

	// Verify known steps are present with summaries
	found := make(map[string]string)
	for _, s := range e.Steps {
		found[s.Kind] = s.Desc
	}

	for _, kind := range []string{"copy", "symlink"} {
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
		wantExample    bool
	}{
		{
			kind:           "copy",
			wantSummary:    "Copy files with owner and permission management",
			wantFields:     []string{"src", "dest", "perm", "owner", "group"},
			wantFieldCount: 6, // includes desc
			wantExample:    true,
		},
		{
			kind:           "symlink",
			wantSummary:    "Create and manage symbolic links",
			wantFields:     []string{"target", "link"},
			wantFieldCount: 3, // includes desc
			wantExample:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

			err := engine.IndexStep(context.Background(), tt.kind, em)
			if err != nil {
				t.Fatalf("IndexStep(%q) failed: %v", tt.kind, err)
			}

			if len(rec.indexStepEvents) == 0 {
				t.Fatal("no IndexStepEvent emitted")
			}

			if len(rec.indexStepEvents) != 1 {
				t.Fatalf("too many IndexStepEvent emitted - have %d, want 1", len(rec.indexStepEvents))
			}

			doc := rec.indexStepEvents[0].Doc

			// Check kind
			if doc.Kind != tt.kind {
				t.Errorf("Kind = %q, want %q", doc.Kind, tt.kind)
			}

			// Check summary
			if doc.Summary != tt.wantSummary {
				t.Errorf("Summary = %q, want %q", doc.Summary, tt.wantSummary)
			}

			// Check field count
			if len(doc.Fields) != tt.wantFieldCount {
				t.Errorf("len(Fields) = %d, want %d", len(doc.Fields), tt.wantFieldCount)
			}

			// Check required fields are present
			fieldNames := make(map[string]bool)
			for _, f := range doc.Fields {
				fieldNames[f.Name] = true

				// Every field should have a type
				if f.Type == "" {
					t.Errorf("field %q has empty Type", f.Name)
				}
			}

			for _, wantField := range tt.wantFields {
				if !fieldNames[wantField] {
					t.Errorf("missing field %q", wantField)
				}
			}

			// Check example
			if tt.wantExample && len(doc.Examples) == 0 {
				t.Error("expected example, got none")
			}
			if tt.wantExample && len(doc.Examples) > 0 && doc.Examples[0] == "" {
				t.Error("example is empty string")
			}
		})
	}
}

func TestIndexStep_UnknownKind_Aborts(t *testing.T) {
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	err := engine.IndexStep(context.Background(), "nonexistent", em)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}

	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Errorf("expected AbortError, got %T", err)
	}
}

func TestIndexStep_FieldsHaveDocumentation(t *testing.T) {
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	_ = engine.IndexStep(context.Background(), "symlink", em)

	if len(rec.indexStepEvents) == 0 {
		t.Fatal("no IndexStepEvent emitted")
	}

	if len(rec.indexStepEvents) != 1 {
		t.Fatalf("too many IndexStepEvent emitted - have %d, want 1", len(rec.indexStepEvents))
	}

	for _, f := range rec.indexStepEvents[0].Doc.Fields {
		if f.Desc == "" {
			t.Errorf("field %q has no @doc description", f.Name)
		}
	}
}

func TestIndexStep_RequiredFieldsMarkedCorrectly(t *testing.T) {
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	_ = engine.IndexStep(context.Background(), "symlink", em)

	if len(rec.indexStepEvents) == 0 {
		t.Fatal("no IndexStepEvent emitted")
	}

	if len(rec.indexStepEvents) != 1 {
		t.Fatalf("too many IndexStepEvent emitted - have %d, want 1", len(rec.indexStepEvents))
	}

	fields := make(map[string]bool)
	for _, f := range rec.indexStepEvents[0].Doc.Fields {
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
