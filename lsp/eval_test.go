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
	content := "def broken(\n"
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
	content := "x = 42\n"
	s.docs.Open(docURI, content, 1)

	diags := s.evaluate(context.Background(), docURI, content)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestEvaluateMissingRequiredArg(t *testing.T) {
	s := testServer()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "test.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	content := "target.local(name=\"test\")\ndeploy(name=\"d\", targets=[\"test\"], steps=[service()])\n"
	s.docs.Open(docURI, content, 1)

	diags := s.evaluate(context.Background(), docURI, content)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for service() missing name")
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

func TestEvaluateSecretErrorDowngradedToHint(t *testing.T) {
	dir := t.TempDir()

	secretsJSON := `{"key": "AGE[notreal]"}`
	if err := os.WriteFile(filepath.Join(dir, "secrets.age.json"), []byte(secretsJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "test.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	content := "secrets(backend=\"age\", path=\"secrets.age.json\")\n"
	s.docs.Open(docURI, content, 1)

	diags := s.evaluate(context.Background(), docURI, content)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for secret decryption failure")
	}
	for _, d := range diags {
		if d.Severity == protocol.DiagnosticSeverityError {
			t.Errorf("secret errors should be hints, not errors: %s", d.Message)
		}
	}
}

func TestEvaluateWithModuleResolution(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "mylib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}

	libContent := "def helper():\n    return 42\n"
	if err := os.WriteFile(filepath.Join(libDir, "mylib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}

	modContent := "module test.example/proj\n\nrequire (\n\tmylib ./mylib\n)\n"
	if err := os.WriteFile(filepath.Join(dir, "scampi.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.rootDir = dir
	s.loadModule()

	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	content := "load(\"mylib\", \"helper\")\nx = helper()\n"
	s.docs.Open(docURI, content, 1)

	diags := s.evaluate(context.Background(), docURI, content)
	for _, d := range diags {
		if d.Severity == protocol.DiagnosticSeverityError {
			t.Errorf("module load should resolve without errors: %s", d.Message)
		}
	}
}

func TestDiagnoseWorkspace(t *testing.T) {
	dir := t.TempDir()

	good := "x = 42\n"
	bad := "def broken(\n"
	if err := os.WriteFile(filepath.Join(dir, "good.scampi"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.scampi"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.rootDir = dir

	// diagnoseWorkspace needs a client to publish to. Use a recording client.
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
