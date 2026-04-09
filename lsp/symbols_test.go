// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
)

func TestDocumentSymbolFunctions(t *testing.T) {
	s := testServer()
	text := "module main\n\nfunc greet(name: string) string {\n  return \"\"\n}\n\nfunc farewell() string {\n  return \"\"\n}\n"
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
	text := "module main\n\nlet config = \"value\"\nlet count = 42\n"
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

func TestWorkspaceSymbols(t *testing.T) {
	dir := t.TempDir()

	f1 := "module a\n\nfunc greet(name: string) string {\n  return \"\"\n}\n"
	f2 := "module b\n\nfunc farewell() string {\n  return \"\"\n}\n\nlet count = 0\n"
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

	content := "module test\n\nfunc greet() string {\n  return \"\"\n}\n\nfunc goodbye() string {\n  return \"\"\n}\n"
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
