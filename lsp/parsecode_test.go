// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

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
