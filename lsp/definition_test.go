// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"testing"

	"go.lsp.dev/protocol"
)

func TestDefinitionLetBinding(t *testing.T) {
	s := testServer()
	text := "module main\n\nlet x = 42\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	// Cursor on "x" at line 2
	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 4},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected definition location")
	}
	if locs[0].Range.Start.Line != 2 {
		t.Errorf("expected definition at line 2, got %d", locs[0].Range.Start.Line)
	}
}

func TestDefinitionFuncDecl(t *testing.T) {
	s := testServer()
	text := "module main\n\nfunc greet(name: string) string {\n  return \"\"\n}\n\ngreet(name = \"world\")\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	// Cursor on "greet" at line 6 (the call)
	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 6, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected definition location")
	}
	if locs[0].Range.Start.Line != 2 {
		t.Errorf("expected definition at line 2, got %d", locs[0].Range.Start.Line)
	}
}

func TestDefinitionUFCSResolvesToStub(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

import "std/secrets"

let age = secrets.from_age(path = "s.json")
let v = age.get("key")
`
	s.docs.Open(docURI, text, 1)

	// Cursor on "get" in "age.get" at line 6, character 12
	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 6, Character: 12},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected UFCS definition to resolve to secrets.get stub")
	}
	if locs[0].URI == "" {
		t.Error("expected non-empty URI for secrets.get stub")
	}
}

func TestDefinitionStdlibResolvesToStub(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "posix.copy", 1)

	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 8},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected stdlib definition to resolve to stub file")
	}
	if locs[0].URI == "" {
		t.Error("expected non-empty URI")
	}
}
func definitionAt(t *testing.T, s *Server, docURI protocol.DocumentURI, line, col uint32) []protocol.Location {
	t.Helper()
	result, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: col},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDefinition_FuncDecl(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

func add(a: int, b: int) int {
  return a + b
}

let r = add(1, 2)
`
	s.docs.Open(docURI, text, 1)

	locs := definitionAt(t, s, docURI, 7, 9)
	if len(locs) == 0 {
		t.Fatal("expected definition location for 'add'")
	}
	if locs[0].Range.Start.Line != 3 {
		t.Errorf("definition line = %d, want 3", locs[0].Range.Start.Line)
	}
}

func TestDefinition_TypeDecl(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `
module main

type Server {
  name: string
}

let s = Server { name = "web" }
`
	s.docs.Open(docURI, text, 1)

	// Goto def on "Server" in the struct literal.
	locs := definitionAt(t, s, docURI, 7, 10)
	if len(locs) == 0 {
		t.Fatal("expected definition location for 'Server'")
	}
	if locs[0].Range.Start.Line != 3 {
		t.Errorf("definition line = %d, want 3", locs[0].Range.Start.Line)
	}
}

func TestDefinition_StdlibFunc(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `posix.copy`
	s.docs.Open(docURI, text, 1)

	locs := definitionAt(t, s, docURI, 0, 8)
	// Should jump to the stub file.
	if len(locs) == 0 {
		t.Log("no definition for stdlib func (stub defs may not be available in test)")
	}
}

func TestDefinition_NoDoc(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///nonexistent.scampi")

	locs := definitionAt(t, s, docURI, 0, 0)
	if len(locs) != 0 {
		t.Error("expected no locations for nonexistent document")
	}
}
