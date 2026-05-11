// SPDX-License-Identifier: GPL-3.0-only

// Most hover-on-user-type tests migrated to fixtures:
//   testdata/hover/user_type_fields (covers fields + defaults)

package lsp

import (
	"context"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestHoverUserDefinedTypeAtDecl(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `module main

type Server {
  name: string
  port: int
}
`
	s.docs.Open(docURI, text, 1)

	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 7},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover result for type declaration")
	}

	md := result.Contents.Value
	if !strings.Contains(md, "name") {
		t.Error("hover should list field 'name'")
	}
	if !strings.Contains(md, "port") {
		t.Error("hover should list field 'port'")
	}
}
