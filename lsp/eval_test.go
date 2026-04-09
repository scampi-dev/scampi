// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func TestEvaluateSyntaxError(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	content := "module main\n@@@\n"
	s.docs.Open(docURI, content, 1)

	diags := s.evaluate(context.Background(), docURI, content)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for syntax error")
	}
	if diags[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("expected error severity, got %v", diags[0].Severity)
	}
}

func TestEvaluateValidConfig(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	content := "module main\n\nlet x = 42\n"
	s.docs.Open(docURI, content, 1)

	diags := s.evaluate(context.Background(), docURI, content)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestEvaluateTypeError(t *testing.T) {
	s := testServer()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "test.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	content := "module main\n\nimport \"std\"\nimport \"std/posix\"\n\nlet t = posix.local { name = \"test\" }\nstd.deploy(name = \"d\", targets = [t]) {\n  posix.service { name = 42 }\n}\n"
	s.docs.Open(docURI, content, 1)

	diags := s.evaluate(context.Background(), docURI, content)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for type error")
	}

	found := false
	for _, d := range diags {
		if d.Severity == protocol.DiagnosticSeverityError {
			found = true
		}
	}
	if !found {
		t.Error("expected an error-severity diagnostic")
	}
}

func TestDiagnoseWorkspace(t *testing.T) {
	dir := t.TempDir()

	good := "module main\n\nlet x = 42\n"
	bad := "module main\n@@@\n"
	if err := os.WriteFile(filepath.Join(dir, "good.scampi"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.scampi"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.rootDir = dir

	published := make(map[protocol.DocumentURI][]protocol.Diagnostic)
	s.client = &recordingClient{published: published}

	s.diagnoseWorkspace(context.Background())

	goodURI := protocol.DocumentURI(uri.File(filepath.Join(dir, "good.scampi")))
	badURI := protocol.DocumentURI(uri.File(filepath.Join(dir, "bad.scampi")))

	if _, ok := published[goodURI]; !ok {
		t.Error("expected diagnostics published for good.scampi")
	}
	if _, ok := published[badURI]; !ok {
		t.Error("expected diagnostics published for bad.scampi")
	}
	if len(published[badURI]) == 0 {
		t.Error("expected errors for bad.scampi")
	}
}

// recordingClient captures PublishDiagnostics calls for testing.
type recordingClient struct {
	protocol.Client
	published map[protocol.DocumentURI][]protocol.Diagnostic
}

func (c *recordingClient) PublishDiagnostics(_ context.Context, params *protocol.PublishDiagnosticsParams) error {
	c.published[params.URI] = params.Diagnostics
	return nil
}
