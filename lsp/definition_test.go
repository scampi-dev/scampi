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

func TestDefinitionLocalAssignment(t *testing.T) {
	s := testServer()
	text := "x = 42\nprint(x)\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	// Cursor on "x" at line 1 (the usage in print(x))
	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected definition location")
	}
	if locs[0].Range.Start.Line != 0 {
		t.Errorf("expected definition at line 0, got %d", locs[0].Range.Start.Line)
	}
}

func TestDefinitionLocalDef(t *testing.T) {
	s := testServer()
	text := "def greet(name):\n    pass\n\ngreet(\"world\")\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	// Cursor on "greet" at line 3 (the call)
	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 3, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected definition location")
	}
	if locs[0].Range.Start.Line != 0 {
		t.Errorf("expected definition at line 0, got %d", locs[0].Range.Start.Line)
	}
}

func TestDefinitionLoadModulePath(t *testing.T) {
	dir := t.TempDir()

	// Write a library file.
	libContent := "def helper():\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainContent := "load(\"lib.scampi\", \"helper\")\nhelper()\n"
	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	s.docs.Open(docURI, mainContent, 1)

	// Cursor on the module path string "lib.scampi" — line 0, inside the quotes
	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 7},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected definition for load module path")
	}
	if locs[0].URI != protocol.DocumentURI(uri.File(filepath.Join(dir, "lib.scampi"))) {
		t.Errorf("expected definition in lib.scampi, got %s", locs[0].URI)
	}
}

func TestDefinitionLoadSymbol(t *testing.T) {
	dir := t.TempDir()

	libContent := "def helper():\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainContent := "load(\"lib.scampi\", \"helper\")\nhelper()\n"
	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	s.docs.Open(docURI, mainContent, 1)

	// Cursor on "helper" at line 1 (the call)
	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected definition for loaded symbol")
	}
	libURI := protocol.DocumentURI(uri.File(filepath.Join(dir, "lib.scampi")))
	if locs[0].URI != libURI {
		t.Errorf("expected definition in lib.scampi, got %s", locs[0].URI)
	}
	if locs[0].Range.Start.Line != 0 {
		t.Errorf("expected definition at line 0, got %d", locs[0].Range.Start.Line)
	}
}

func TestDefinitionViaModuleResolution(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "mylib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}

	libContent := "def helper():\n    pass\n"
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
	mainContent := "load(\"mylib\", \"helper\")\nhelper()\n"
	s.docs.Open(docURI, mainContent, 1)

	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Fatal("expected definition via module resolution")
	}
	libURI := protocol.DocumentURI(uri.File(filepath.Join(libDir, "mylib.scampi")))
	if locs[0].URI != libURI {
		t.Errorf("expected definition in mylib.scampi, got %s", locs[0].URI)
	}
}

func TestDefinitionBuiltinReturnsNil(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "copy", 1)

	locs, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 0 {
		t.Error("builtins have no source definition, expected empty result")
	}
}
