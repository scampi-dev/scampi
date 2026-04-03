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

func TestReferencesLocalIdent(t *testing.T) {
	s := testServer()
	text := "x = 42\ny = x + 1\nprint(x)\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// x appears 3 times: assignment, usage in y, usage in print
	if len(locs) != 3 {
		t.Errorf("expected 3 references for x, got %d", len(locs))
	}
}

func TestReferencesNoDuplicatesFromLoad(t *testing.T) {
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

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Dedup should prevent load()'s To/From duplication.
	// Expect: 1 in load statement (To), 1 call on line 1, 1 def in lib.scampi
	seen := make(map[string]int)
	for _, loc := range locs {
		key := string(loc.URI) + ":" + formatPos(loc.Range.Start)
		seen[key]++
	}
	for key, count := range seen {
		if count > 1 {
			t.Errorf("duplicate reference at %s (%d times)", key, count)
		}
	}
}

func TestReferencesAcrossFiles(t *testing.T) {
	dir := t.TempDir()

	libContent := "def greet(name):\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}
	mainContent := "load(\"lib.scampi\", \"greet\")\ngreet(\"world\")\n"
	if err := os.WriteFile(filepath.Join(dir, "main.scampi"), []byte(mainContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	s.docs.Open(docURI, mainContent, 1)

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should find refs in both main.scampi and lib.scampi
	files := make(map[protocol.DocumentURI]bool)
	for _, loc := range locs {
		files[loc.URI] = true
	}

	libURI := protocol.DocumentURI(uri.File(filepath.Join(dir, "lib.scampi")))
	if !files[docURI] {
		t.Error("expected references in main.scampi")
	}
	if !files[libURI] {
		t.Error("expected references in lib.scampi")
	}
}

func TestReferencesBuiltinReturnsNil(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "copy", 1)

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 0 {
		t.Error("builtins should not return references")
	}
}

func formatPos(p protocol.Position) string {
	return string(rune('0'+p.Line)) + ":" + string(rune('0'+p.Character))
}
