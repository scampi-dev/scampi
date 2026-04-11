// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestCompletionUFCSNestedScope — UFCS completion for receivers
// declared inside a function body. The `let x = b()` lives in
// `bb`'s ScopeFunc which gets popped after checking, so the LSP
// looks the receiver up via the checker's flat AllBindings fallback
// map. Also exercises isFunctionBodyBrace, which prevents the body
// brace from being mis-classified as a struct-literal context.
func TestCompletionUFCSNestedScope(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.scampi")

	s := testServer()
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := `module main

type X {
  name: string
}

func a(x: X) string {
  return ""
}

func b() X {
  return X { name = "hello" }
}

func bb() string {
  let x = b()
  x.
}
`
	s.docs.Open(docURI, text, 1)

	// Cursor right after `x.` on line 16, col 4
	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 16, Character: 4},
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
	t.Logf("got %d items: %v", len(result.Items), labels)
	if !labels["a"] {
		t.Errorf("expected UFCS function 'a' on x: X, got %v", labels)
	}
}
