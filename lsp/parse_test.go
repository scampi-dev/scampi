// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"testing"

	"go.lsp.dev/protocol"
)

func TestParseValidSource(t *testing.T) {
	src := "module main\n\nlet x = 1\nlet y = \"hello\"\n"
	f, diags := Parse("test.scampi", []byte(src))
	if len(diags) > 0 {
		t.Errorf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
	if f == nil {
		t.Error("expected non-nil AST")
	}
}

func TestParseSyntaxError(t *testing.T) {
	src := "module main\n@@@"
	_, diags := Parse("test.scampi", []byte(src))
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for syntax error")
	}
	for _, d := range diags {
		if d.Severity != protocol.DiagnosticSeverityError {
			t.Errorf("expected error severity, got %v", d.Severity)
		}
		if d.Source != "scampi" {
			t.Errorf("expected source 'scampi', got %q", d.Source)
		}
	}
}

func TestParseDeclarations(t *testing.T) {
	src := "module main\n\nfunc greet(name: string) string {\n  return \"\"\n}\n\nlet count = 42\n"
	f, diags := Parse("test.scampi", []byte(src))
	if len(diags) > 0 {
		t.Errorf("expected no diagnostics: %v", diags)
	}
	if f == nil {
		t.Fatal("expected non-nil AST")
	}
	if len(f.Decls) != 2 {
		t.Errorf("expected 2 declarations, got %d", len(f.Decls))
	}
}

func TestParseDiagnosticPosition(t *testing.T) {
	src := "module main\n@@@\n"
	_, diags := Parse("test.scampi", []byte(src))
	if len(diags) == 0 {
		t.Fatal("expected diagnostics")
	}
	d := diags[0]
	if d.Range.Start.Line < 1 {
		t.Errorf("expected error on line >= 1 (0-indexed), got line %d", d.Range.Start.Line)
	}
}
