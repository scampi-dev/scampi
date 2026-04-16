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
	content := `module main

import "std"
import "std/posix"
import "std/local"

let t = local.target { name = "test" }
std.deploy(name = "d", targets = [t]) {
  posix.service { name = 42 }
}
`
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

func TestEvaluateBadSecretKey(t *testing.T) {
	// Tests that the LSP surfaces a diagnostic for a literal secret
	// key that doesn't exist in the configured backend. The check is
	// driven by the `@secretkey` attribute on `func secret`'s `name`
	// parameter and the linker's static-check pass that runs as part
	// of linker.Analyze.
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "secrets.json")
	if err := os.WriteFile(
		secretsPath,
		[]byte(`{"db.password": "encrypted"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "test.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	content := `module main

import "std"

std.secrets { backend = std.SecretsBackend.file, path = "secrets.json" }

let v = std.secret("totally.does.not.exist")
`
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.docs.Open(docURI, content, 1)
	diags := s.evaluate(context.Background(), docURI, content)
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic for bad secret key")
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

func (c *recordingClient) WorkDoneProgressCreate(context.Context, *protocol.WorkDoneProgressCreateParams) error {
	return nil
}

func (c *recordingClient) Progress(context.Context, *protocol.ProgressParams) error {
	return nil
}
