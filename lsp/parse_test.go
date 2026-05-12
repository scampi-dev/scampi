// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
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

func TestParseErrors_HaveStableCodes(t *testing.T) {
	s := testServer()

	tests := []struct {
		name     string
		src      string
		wantCode string
	}{
		{
			name:     "missing module decl",
			src:      "let x = 1\n",
			wantCode: "parse.MissingModuleDecl",
		},
		{
			name:     "unterminated string",
			src:      "module main\nlet x = \"hello\n",
			wantCode: "lex.UnterminatedString",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docURI := protocol.DocumentURI(uri.File("/test/parsecode_" + tt.name + ".scampi"))
			s.docs.Open(docURI, tt.src, 1)

			_, diags := Parse(uriToPath(docURI), []byte(tt.src))
			if len(diags) == 0 {
				t.Fatal("expected diagnostics")
			}

			code, _ := diags[0].Code.(string)
			if code != tt.wantCode {
				t.Errorf("expected code %q, got %q (msg: %s)", tt.wantCode, code, diags[0].Message)
			}
		})
	}
}

func TestParseErrors_ExpectedTokenCodes(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantCode string
	}{
		{
			name:     "expected rbrace in type body",
			src:      "module main\ntype Foo {\n",
			wantCode: "parse.ExpectedRBrace",
		},
		{
			name:     "expected string in import",
			src:      "module main\nimport foo\n",
			wantCode: "parse.ExpectedString",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := Parse("test.scampi", []byte(tt.src))
			if len(diags) == 0 {
				t.Fatal("expected diagnostics")
			}

			found := false
			for _, d := range diags {
				code, _ := d.Code.(string)
				if code == tt.wantCode {
					found = true
					break
				}
			}
			if !found {
				codes := make([]string, len(diags))
				for i, d := range diags {
					codes[i], _ = d.Code.(string)
				}
				t.Errorf("expected code %q in diagnostics, got %v", tt.wantCode, codes)
			}
		})
	}
}
