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
		catalog:  NewCatalog(),
		modules:  bootstrapModules(),
		stubDefs: NewStubDefs(),
		docs:     NewDocuments(),
		log:      log.New(io.Discard, "", 0),
	}
}

func TestCompletionTopLevel(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	// Top-level completion: modules are offered as namespaces.
	s.docs.Open(docURI, "pos", 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected completion items for 'pos'")
	}

	found := false
	for _, item := range result.Items {
		if item.Label == "posix" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'posix' module in completion items")
	}
}

func TestCompletionKwargs(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	// Cursor inside a call to posix.copy: posix.copy { src = posix.source_local { path = "./f" }, |
	text := `posix.copy { src = posix.source_local { path = "./f" }, `
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
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "posix.", 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 6},
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
	for _, want := range []string{"copy", "dir", "service"} {
		if !labels[want] {
			t.Errorf("missing posix.%s in completions", want)
		}
	}
}

func TestCompletionEnumValues(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `posix.service { name = "nginx", state = "`
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
	text := `posix.copy { src = `
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
	for _, want := range []string{"posix.source_local", "posix.source_inline", "posix.source_remote"} {
		if !labels[want] {
			t.Errorf("missing source resolver: %s", want)
		}
	}
}

func TestCompletionStringKwargOffersSecretAndEnv(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := "ssh.target {\n    name = \"test\",\n    host = "
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 11},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) == 0 {
		t.Fatal("expected std.secret/std.env completions for string kwarg")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["std.secret"] {
		t.Error("expected 'std.secret' completion for string kwarg")
	}
	if !labels["std.env"] {
		t.Error("expected 'std.env' completion for string kwarg")
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
	text := "std.secrets { backend = \"age\", path = \"secrets.age.json\" }\nstd.secret(\""
	s.docs.Open(docURI, text, 1)

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 12},
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

	libContent := `module lib

func proxy_host(domain: string, forward_host: string, forward_port: int = 443) string {
  return ""
}
`
	if err := os.WriteFile(filepath.Join(dir, "lib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	// User-defined function in same file
	text := `module main

func proxy_host(domain: string, forward_host: string, forward_port: int = 443) string {
  return ""
}

proxy_host()
`
	s.docs.Open(docURI, text, 1)

	// Cursor between the parens on line 6, col 11
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 6, Character: 11},
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
	for _, want := range []string{"domain", "forward_host", "forward_port"} {
		if !labels[want] {
			t.Errorf("missing kwarg: %s", want)
		}
	}
}

// TestCompletionTopLevelIncludesUserDecls — completion at the start
// of an expression should include user-defined funcs/types/lets from
// the current document, not just stdlib catalog members. This is
// the path that was empty when typing `b` before `b()` in the
// user-reported repro.
func TestCompletionTopLevelIncludesUserDecls(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.scampi")

	s := testServer()
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := `module main

type X {
  name: string
}

func b() X {
  return X { name = "hello" }
}

func bb() string {
  b
}
`
	s.docs.Open(docURI, text, 1)

	// Cursor right after `b` on line 11 (`  b`)
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 11, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["b"] {
		t.Errorf("expected user-defined func 'b' in top-level completion, got %v", labels)
	}
}

// TestCompletionUFCS — when the user types `var.` where `var` is a
// let-bound variable, completion should offer free functions in
// scope whose first parameter accepts the variable's type. Mirrors
// the lang/check resolution rules in resolveCallee.
func TestCompletionUFCS(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.scampi")

	s := testServer()
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := `module main

func double(n: int) int {
  return n + n
}

func inc(n: int) int {
  return n + 1
}

func shout(s: string) string {
  return s
}

let n = 5
n.
`
	s.docs.Open(docURI, text, 1)

	// Cursor right after `n.` on the last (line 15) — empty prefix
	// expecting `double` and `inc` (both take int as first arg) but
	// NOT `shout` (takes string).
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 15, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected completion result")
	}

	labels := make(map[string]bool)
	for _, item := range result.Items {
		labels[item.Label] = true
	}
	if !labels["double"] {
		t.Errorf("missing UFCS completion: double")
	}
	if !labels["inc"] {
		t.Errorf("missing UFCS completion: inc")
	}
	if labels["shout"] {
		t.Errorf("shout should NOT be offered (string param, int receiver)")
	}
}
