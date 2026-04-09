// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestHoverOnFuncName(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	// Hover on "posix.copy" — word extraction gives "posix.copy"
	s.docs.Open(docURI, "posix.copy", 1)

	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 8},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover result")
	}
	if !strings.Contains(result.Contents.Value, "posix.copy") {
		t.Errorf("hover should mention 'posix.copy', got %q", result.Contents.Value)
	}
	if !strings.Contains(result.Contents.Value, "src") {
		t.Error("hover should include parameter names")
	}
}

func TestHoverOnKwarg(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `posix.copy { dest = "/etc/foo" }`
	s.docs.Open(docURI, text, 1)

	// Hover on "dest" (inside braces)
	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 15},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover result for kwarg")
	}
	if !strings.Contains(result.Contents.Value, "dest") {
		t.Errorf("hover should mention 'dest', got %q", result.Contents.Value)
	}
}

func TestHoverOnUnknown(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "foobar", 1)

	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil hover for unknown word")
	}
}

func TestHoverOnDottedFunc(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "posix.ssh", 1)

	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 8},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover for posix.ssh")
	}
	if !strings.Contains(result.Contents.Value, "posix.ssh") {
		t.Errorf("hover should mention 'posix.ssh', got %q", result.Contents.Value)
	}
}

func TestHoverUserDefinedFunc(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := "module main\n\nfunc proxy_host(domain: string, forward_host: string, forward_port: int = 443) string {\n  return \"\"\n}\n\nproxy_host(domain = \"test\")\n"
	s.docs.Open(docURI, text, 1)

	// Hover on "proxy_host" at line 6
	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 6, Character: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover for user-defined function")
	}
	if !strings.Contains(result.Contents.Value, "proxy_host") {
		t.Errorf("hover should mention 'proxy_host', got %q", result.Contents.Value)
	}
	if !strings.Contains(result.Contents.Value, "domain") {
		t.Error("hover should include param 'domain'")
	}
	if !strings.Contains(result.Contents.Value, "forward_port") {
		t.Error("hover should include param 'forward_port'")
	}
}
