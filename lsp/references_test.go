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
	text := "module main\n\nlet x = 42\nlet y = x + 1\n"
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 4},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) < 2 {
		t.Errorf("expected at least 2 references for x, got %d", len(locs))
	}
}

func TestReferencesInStubFile(t *testing.T) {
	s := testServer()
	stub := `module posix

import "std"

type Source

decl pkg(
  packages: list[string],
  source: PkgSource,
) std.Step
`
	docURI := protocol.DocumentURI("file:///stubs/posix.scampi")
	s.docs.Open(docURI, stub, 1)

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 6, Character: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		t.Error("expected at least 1 reference for pkg in stub file")
	}
}

func TestReferencesBareDeclInRealStub(t *testing.T) {
	// Use the actual full posix stub content.
	stubContent, err := os.ReadFile("../std/posix/posix.scampi")
	if err != nil {
		t.Skip("could not read posix stub:", err)
	}

	s := testServer()
	dir := t.TempDir()
	stubPath := filepath.Join(dir, "posix.scampi")
	_ = os.WriteFile(stubPath, stubContent, 0o644)
	docURI := protocol.DocumentURI(uri.File(stubPath))
	s.docs.Open(docURI, string(stubContent), 1)

	// Find "pkg" in "decl pkg(" — scan for it.
	text := string(stubContent)
	target := "decl pkg("
	idx := 0
	line := uint32(0)
	for i, ch := range text {
		if i+len(target) <= len(text) && text[i:i+len(target)] == target {
			idx = i
			break
		}
		if ch == '\n' {
			line++
		}
	}
	// cursor on "pkg" = 5 chars into "decl pkg("
	col := uint32(idx - lineStart(text, idx) + 5)

	t.Logf("searching for bare 'pkg' at line=%d col=%d", line, col)

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: col},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 {
		// Debug: check what word was resolved
		word := wordAtOffset(text, offsetFromPosition(text, line, col))
		t.Errorf("expected at least 1 reference for 'pkg', wordAtOffset=%q", word)
	} else {
		t.Logf("found %d references", len(locs))
	}
}

func TestReferencesStdlibFromConfig(t *testing.T) {
	s := testServer()

	config := `module main

import "std"
import "std/posix"
import "std/local"

let t = local.target { name = "local" }

std.deploy(name = "test", targets = [t]) {
  posix.pkg {
    packages = ["nginx"]
    source = posix.pkg_system {}
  }
}
`
	docURI := protocol.DocumentURI("file:///test/config.scampi")
	s.docs.Open(docURI, config, 1)

	// Find references for "posix.pkg" — cursor on "pkg" in "posix.pkg {"
	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 8, Character: 10},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should find the usage in config + the stub def.
	if len(locs) < 2 {
		t.Errorf("expected at least 2 references (usage + stub def), got %d", len(locs))
		for _, l := range locs {
			t.Logf("  %s L%d:%d", l.URI, l.Range.Start.Line, l.Range.Start.Character)
		}
	}
}

func TestReferencesStdStepInStub(t *testing.T) {
	// Simulate: user opened posix stub, cursor on "std.Step" return type.
	// "std.Step" appears many times in the posix stub as return types.
	s := testServer()

	dir := t.TempDir()
	stubContent, _ := os.ReadFile(filepath.Join(
		os.Getenv("HOME"),
		"Library",
		"Caches",
		"scampls",
		"stubs",
		"v0.0.0-dev",
		"posix",
		"posix.scampi",
	))
	if len(stubContent) == 0 {
		// Fallback: use the source stub directly.
		stubContent, _ = os.ReadFile("../std/posix/posix.scampi")
	}
	if len(stubContent) == 0 {
		t.Skip("could not read posix stub")
	}

	stubPath := filepath.Join(dir, "posix.scampi")
	_ = os.WriteFile(stubPath, stubContent, 0o644)
	docURI := protocol.DocumentURI(uri.File(stubPath))
	s.docs.Open(docURI, string(stubContent), 1)

	// Find "std.Step" — it's a return type on many decls.
	// Find the first occurrence by scanning for it.
	text := string(stubContent)
	line := uint32(0)
	col := uint32(0)
	for i, ch := range text {
		if i+8 <= len(text) && text[i:i+8] == "std.Step" {
			col = uint32(i - lineStart(text, i) + 4) // cursor on "Step"
			break
		}
		if ch == '\n' {
			line++
		}
	}

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: line, Character: col},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// posix stub has ~15 decls returning std.Step
	if len(locs) < 5 {
		t.Errorf("expected many references for std.Step in posix stub, got %d", len(locs))
	}
}

// References on attribute types
// -----------------------------------------------------------------------------

func TestReferencesAttrTypeFromDef(t *testing.T) {
	// Cursor on `secretkey` in `type @secretkey {}` should return:
	//   - the def site itself
	//   - every Attribute reference using `@secretkey` in this file
	s := testServer()
	text := `
module main
type @secretkey {}

func secret(@secretkey name: string) string

let v = secret("foo")
`
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	// Cursor on `secretkey` in the type decl (line 1, col ~7).
	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 7},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) < 2 {
		t.Errorf(
			"expected at least 2 references for @secretkey (def + 1 use), got %d",
			len(locs),
		)
	}
}

func TestReferencesAttrTypeFromUse(t *testing.T) {
	// Cursor on `@secretkey` use should also find both the def and
	// the use itself.
	s := testServer()
	text := `
module main
type @secretkey {}

func secret(@secretkey name: string) string
`
	docURI := protocol.DocumentURI("file:///test.scampi")
	s.docs.Open(docURI, text, 1)

	// Cursor inside `@secretkey` of the param annotation.
	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 4, Character: 16},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) < 2 {
		t.Errorf(
			"expected at least 2 references for @secretkey from use site, got %d",
			len(locs),
		)
	}
}

func TestReferencesUFCSFromStub(t *testing.T) {
	// Cursor on "get" in a module stub. User files call it as
	// `age.get("key")` (UFCS), not `secrets.get(...)`. The
	// reference search must find these UFCS call sites.
	dir := t.TempDir()

	stub := `module secrets

type SecretResolver
func get(resolver: SecretResolver, key: string) string
`
	config := `module main

import "std/secrets"

let age = secrets.from_age(path = "s.json")
let v = age.get("key")
`
	if err := os.WriteFile(filepath.Join(dir, "secrets.scampi"), []byte(stub), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.scampi"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	s := testServer()
	s.rootDir = dir

	stubPath := filepath.Join(dir, "secrets.scampi")
	configPath := filepath.Join(dir, "config.scampi")
	stubURI := protocol.DocumentURI(uri.File(stubPath))
	configURI := protocol.DocumentURI(uri.File(configPath))
	s.docs.Open(stubURI, stub, 1)
	s.docs.Open(configURI, config, 1)

	// Cursor on "get" in `func get(` at line 3, character 5.
	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: stubURI},
			Position:     protocol.Position{Line: 3, Character: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, loc := range locs {
		if loc.URI == configURI {
			found = true
		}
	}
	if !found {
		t.Errorf("expected UFCS reference in config file, got %d locations:", len(locs))
		for _, l := range locs {
			t.Logf("  %s L%d:%d", l.URI, l.Range.Start.Line, l.Range.Start.Character)
		}
	}
}

func lineStart(text string, pos int) int {
	for i := pos - 1; i >= 0; i-- {
		if text[i] == '\n' {
			return i + 1
		}
	}
	return 0
}
