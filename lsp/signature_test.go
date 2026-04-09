// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestSignatureHelp(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `std.deploy(`
	s.docs.Open(docURI, text, 1)

	result, err := s.SignatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: uint32(len(text))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Signatures) == 0 {
		t.Fatal("expected signature help")
	}

	sig := result.Signatures[0]
	if !strings.HasPrefix(sig.Label, "std.deploy(") {
		t.Errorf("expected signature starting with 'std.deploy(', got %q", sig.Label)
	}
	if len(sig.Parameters) == 0 {
		t.Error("expected parameters")
	}
}

func TestSignatureHelpActiveParam(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `std.deploy(name = "d", `
	s.docs.Open(docURI, text, 1)

	result, err := s.SignatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: uint32(len(text))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected signature help")
	}
	if result.ActiveParameter != 1 {
		t.Errorf("active param = %d, want 1", result.ActiveParameter)
	}
}

func TestSignatureHelpOutsideCall(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "let x = 1", 1)

	result, err := s.SignatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 9},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil signature help outside a call")
	}
}

func TestSignatureHelpUserDefinedFunc(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := "module main\n\nfunc proxy_host(domain: string, forward_host: string, forward_port: int = 443) string {\n  return \"\"\n}\n\nproxy_host()\n"
	s.docs.Open(docURI, text, 1)

	result, err := s.SignatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 6, Character: 11},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Signatures) == 0 {
		t.Fatal("expected signature help for user-defined function")
	}
	sig := result.Signatures[0]
	if !strings.HasPrefix(sig.Label, "proxy_host(") {
		t.Errorf("expected signature starting with 'proxy_host(', got %q", sig.Label)
	}
	if len(sig.Parameters) != 3 {
		t.Errorf("expected 3 params, got %d", len(sig.Parameters))
	}
}
