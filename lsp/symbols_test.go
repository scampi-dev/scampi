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

func TestDocumentSymbolFunctions(t *testing.T) {
	s := testServer()
	text := "def greet(name):\n    pass\n\ndef farewell():\n    pass\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	result, err := s.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(result))
	}

	names := make(map[string]bool)
	for _, sym := range result {
		ds, ok := sym.(protocol.DocumentSymbol)
		if !ok {
			t.Fatalf("expected DocumentSymbol, got %T", sym)
		}
		names[ds.Name] = true
		if ds.Kind != protocol.SymbolKindFunction {
			t.Errorf("expected function kind for %s, got %v", ds.Name, ds.Kind)
		}
	}
	if !names["greet"] {
		t.Error("missing symbol: greet")
	}
	if !names["farewell"] {
		t.Error("missing symbol: farewell")
	}
}

func TestDocumentSymbolVariables(t *testing.T) {
	s := testServer()
	text := "config = {\"key\": \"value\"}\ncount = 42\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	result, err := s.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(result))
	}

	for _, sym := range result {
		ds := sym.(protocol.DocumentSymbol)
		if ds.Kind != protocol.SymbolKindVariable {
			t.Errorf("expected variable kind for %s, got %v", ds.Name, ds.Kind)
		}
	}
}

func TestDocumentSymbolExcludesLoads(t *testing.T) {
	s := testServer()
	text := "load(\"lib.scampi\", \"helper\")\ndef main():\n    pass\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	result, err := s.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 symbol (main only, no load), got %d", len(result))
	}
	ds := result[0].(protocol.DocumentSymbol)
	if ds.Name != "main" {
		t.Errorf("expected 'main', got %q", ds.Name)
	}
}

func TestWorkspaceSymbols(t *testing.T) {
	dir := t.TempDir()

	f1 := "def greet(name):\n    pass\n"
	f2 := "def farewell():\n    pass\ncount = 0\n"
	if err := os.WriteFile(filepath.Join(dir, "a.scampi"), []byte(f1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.scampi"), []byte(f2), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.rootDir = dir

	syms, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{
		Query: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, sym := range syms {
		names[sym.Name] = true
	}
	for _, want := range []string{"greet", "farewell", "count"} {
		if !names[want] {
			t.Errorf("missing workspace symbol: %s", want)
		}
	}
}

func TestWorkspaceSymbolsQueryFilters(t *testing.T) {
	dir := t.TempDir()

	content := "def greet():\n    pass\ndef goodbye():\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "test.scampi"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.rootDir = dir

	syms, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{
		Query: "gre",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol matching 'gre', got %d", len(syms))
	}
	if syms[0].Name != "greet" {
		t.Errorf("expected 'greet', got %q", syms[0].Name)
	}
}

func TestWorkspaceSymbolsIncludesDeps(t *testing.T) {
	dir := t.TempDir()
	depDir := filepath.Join(dir, "deps", "mylib")
	if err := os.MkdirAll(depDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mainContent := "def main_fn():\n    pass\n"
	depContent := "def dep_fn():\n    pass\n"
	modContent := "module test.example/proj\n\nrequire (\n\tmylib ./deps/mylib\n)\n"

	if err := os.WriteFile(filepath.Join(dir, "main.scampi"), []byte(mainContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "lib.scampi"), []byte(depContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scampi.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.rootDir = dir
	s.loadModule()

	syms, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{
		Query: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, sym := range syms {
		names[sym.Name] = true
	}
	if !names["main_fn"] {
		t.Error("missing symbol from workspace: main_fn")
	}
	if !names["dep_fn"] {
		t.Error("missing symbol from dependency: dep_fn")
	}
}

func TestWorkspaceSymbolsNoRootReturnsNil(t *testing.T) {
	s := testServer()

	syms, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{
		Query: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if syms != nil {
		t.Error("expected nil when rootDir is empty")
	}
}

func TestWorkspaceSymbolsExcludesNonScampiFiles(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "test.scampi"), []byte("def found():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\nfunc notfound() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.rootDir = dir
	_ = uri.File(dir) // suppress unused import

	syms, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{Query: ""})
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range syms {
		if sym.Name == "notfound" {
			t.Error("should not scan .go files")
		}
	}
}
