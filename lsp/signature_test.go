// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func TestSignatureHelp(t *testing.T) {
	s := testServer()
	uri := protocol.DocumentURI("file:///test.scampi")
	text := `copy(`
	s.docs.Open(uri, text, 1)

	result, err := s.SignatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
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
	if !strings.HasPrefix(sig.Label, "copy(") {
		t.Errorf("expected signature starting with 'copy(', got %q", sig.Label)
	}
	if len(sig.Parameters) == 0 {
		t.Error("expected parameters")
	}
}

func TestSignatureHelpActiveParam(t *testing.T) {
	s := testServer()
	uri := protocol.DocumentURI("file:///test.scampi")
	text := `copy(src=local("./f"), `
	s.docs.Open(uri, text, 1)

	result, err := s.SignatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
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
	uri := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(uri, "x = 1", 1)

	result, err := s.SignatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 5},
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
	dir := t.TempDir()

	libContent := "def proxy_host(domain, forward_host, forward_port=443):\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := "load(\"lib.scampi\", \"proxy_host\")\nproxy_host()\n"
	s.docs.Open(docURI, text, 1)

	result, err := s.SignatureHelp(context.Background(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 11},
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
