// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func testServer() *Server {
	return &Server{
		catalog: NewCatalog(),
		docs:    NewDocuments(),
		log:     log.New(io.Discard, "", 0),
	}
}

func TestCompletionTopLevel(t *testing.T) {
	s := testServer()
	uri := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(uri, "cop", 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected completion items for 'cop'")
	}

	found := false
	for _, item := range result.Items {
		if item.Label == "copy" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'copy' in completion items")
	}
}

func TestCompletionKwargs(t *testing.T) {
	s := testServer()
	uri := protocol.DocumentURI("file:///test.scampi")
	text := `copy(src=local("./f"), `
	s.docs.Open(uri, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: uint32(len(text))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected kwarg completions")
	}

	// "src" should be excluded since it's already present.
	for _, item := range result.Items {
		if item.Label == "src" {
			t.Error("src should be excluded from completions (already present)")
		}
	}

	// "dest" should be offered.
	found := false
	for _, item := range result.Items {
		if item.Label == "dest" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'dest' in kwarg completions")
	}
}

func TestCompletionModule(t *testing.T) {
	s := testServer()
	uri := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(uri, "target.", 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 7},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected module member completions")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	for _, want := range []string{"ssh", "local", "rest"} {
		if !labels[want] {
			t.Errorf("missing target.%s in completions", want)
		}
	}
}

func TestCompletionEnumValues(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `service(name="nginx", state="`
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: uint32(len(text))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected enum value completions")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	for _, want := range []string{"running", "stopped", "restarted", "reloaded"} {
		if !labels[want] {
			t.Errorf("missing enum value: %s", want)
		}
	}
}

func TestCompletionSourceResolvers(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `copy(src=`
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: uint32(len(text))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected source resolver completions")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	for _, want := range []string{"local", "inline", "remote"} {
		if !labels[want] {
			t.Errorf("missing source resolver: %s", want)
		}
	}
}

func TestCompletionStringKwargOffersSecretAndEnv(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := "target.ssh(\n    name=\"test\",\n    host="
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 9},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected secret/env completions for string kwarg")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["secret"] {
		t.Error("expected 'secret' completion for string kwarg")
	}
	if !labels["env"] {
		t.Error("expected 'env' completion for string kwarg")
	}
}

func TestCompletionSecretKeys(t *testing.T) {
	dir := t.TempDir()

	secretsJSON := `{"db.host": "encrypted1", "db.pass": "encrypted2"}`
	if err := os.WriteFile(filepath.Join(dir, "secrets.age.json"), []byte(secretsJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "test.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := "secrets(backend=\"age\", path=\"secrets.age.json\")\nsecret(\""
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 8},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected secret key completions")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["db.host"] {
		t.Error("missing secret key: db.host")
	}
	if !labels["db.pass"] {
		t.Error("missing secret key: db.pass")
	}
}

func TestCompletionUserDefinedFuncKwargs(t *testing.T) {
	dir := t.TempDir()

	libContent := "def proxy_host(domain, forward_host, forward_port, certificate=None):\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := "load(\"lib.scampi\", \"proxy_host\")\nproxy_host()\n"
	s.docs.Open(docURI, text, 1)

	// Cursor between the parens on line 1, col 11
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 11},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected kwarg completions for user-defined function")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	for _, want := range []string{"domain", "forward_host", "forward_port", "certificate"} {
		if !labels[want] {
			t.Errorf("missing kwarg: %s", want)
		}
	}
}
