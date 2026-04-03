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

func TestHoverOnFuncName(t *testing.T) {
	s := testServer()
	uri := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(uri, "copy", 1)

	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover result")
	}
	if !strings.Contains(result.Contents.Value, "copy") {
		t.Errorf("hover should mention 'copy', got %q", result.Contents.Value)
	}
	if !strings.Contains(result.Contents.Value, "`src`") {
		t.Error("hover should include parameter docs")
	}
}

func TestHoverOnKwarg(t *testing.T) {
	s := testServer()
	uri := protocol.DocumentURI("file:///test.scampi")
	text := `copy(dest="/etc/foo")`
	s.docs.Open(uri, text, 1)

	// Hover on "dest" (chars 5-8)
	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 6},
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
	uri := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(uri, "foobar = 42", 1)

	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
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
	uri := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(uri, "target.ssh", 1)

	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 8},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover for target.ssh")
	}
	if !strings.Contains(result.Contents.Value, "target.ssh") {
		t.Errorf("hover should mention 'target.ssh', got %q", result.Contents.Value)
	}
}

func TestHoverUserDefinedFunc(t *testing.T) {
	dir := t.TempDir()

	libContent := "def proxy_host(domain, forward_host, forward_port=443):\n    pass\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.scampi"), []byte(libContent), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	mainPath := filepath.Join(dir, "main.scampi")
	docURI := protocol.DocumentURI(uri.File(mainPath))
	text := "load(\"lib.scampi\", \"proxy_host\")\nproxy_host(domain=\"test\")\n"
	s.docs.Open(docURI, text, 1)

	// Hover on "proxy_host" at line 1
	result, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 5},
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
