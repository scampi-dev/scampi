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
