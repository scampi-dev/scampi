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
	s.docs.Open(docURI, "ssh.target", 1)

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
		t.Fatal("expected hover for ssh.target")
	}
	if !strings.Contains(result.Contents.Value, "ssh.target") {
		t.Errorf("hover should mention 'ssh.target', got %q", result.Contents.Value)
	}
}

func TestHoverUserDefinedFunc(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `module main

func proxy_host(domain: string, forward_host: string, forward_port: int = 443) string {
  return ""
}

proxy_host(domain = "test")
`
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

func hoverAt(t *testing.T, s *Server, docURI protocol.DocumentURI, line, col uint32) *protocol.Hover {
	t.Helper()
	result, err := s.Hover(context.Background(), &protocol.HoverParams{
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

func requireHoverContains(t *testing.T, result *protocol.Hover, fragments ...string) {
	t.Helper()
	if result == nil {
		t.Fatal("expected hover result, got nil")
	}
	for _, f := range fragments {
		if !strings.Contains(result.Contents.Value, f) {
			t.Errorf("hover should contain %q, got:\n%s", f, result.Contents.Value)
		}
	}
}

func TestHover_StdlibEnum(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, `module main
import "std/pve"

let x = pve.Console.xtermjs
`, 1)

	result := hoverAt(t, s, docURI, 3, 14)
	if result != nil {
		requireHoverContains(t, result, "Console")
	}
}

func TestHover_UserType_NoFieldsOpaque(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `module main

type Opaque
`
	s.docs.Open(docURI, text, 1)

	result := hoverAt(t, s, docURI, 2, 7)
	if result == nil {
		t.Fatal("expected hover for opaque type")
	}
	requireHoverContains(t, result, "Opaque")
}

func TestHover_UserFunc(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `module main

func greet(name: string, loud: bool = false) string {
  return "hi"
}

greet(name = "world")
`
	s.docs.Open(docURI, text, 1)

	result := hoverAt(t, s, docURI, 6, 3)
	requireHoverContains(t, result, "greet", "name", "loud")
}

func TestHover_LetBindingStructType(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `module main

type Box {
  label: string
}

let b = Box { label = "x" }
`
	s.docs.Open(docURI, text, 1)

	result := hoverAt(t, s, docURI, 6, 5)
	requireHoverContains(t, result, "b", "Box")
}

func TestHover_ForLoopVar(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `module main

type Item {
  id: int
}

let items = [Item { id = 1 }]

func use() string {
  for item in items {
    let x = item
  }
  return ""
}
`
	s.docs.Open(docURI, text, 1)

	result := hoverAt(t, s, docURI, 10, 14)
	if result != nil {
		requireHoverContains(t, result, "item")
	}
}

func TestHover_StdlibType(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, "ssh.target", 1)

	result := hoverAt(t, s, docURI, 0, 8)
	requireHoverContains(t, result, "ssh.target")
}

func TestHover_KwargInsideStructLitWithNewlines(t *testing.T) {
	s := testServer()
	docURI := protocol.DocumentURI("file:///test.scampi")
	text := `posix.copy {
  src = posix.source_local { path = "./f" }
  dest = "/etc/foo"
  owner = "root"
}
`
	s.docs.Open(docURI, text, 1)

	result := hoverAt(t, s, docURI, 3, 4)
	requireHoverContains(t, result, "owner")
}

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
