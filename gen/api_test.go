// SPDX-License-Identifier: GPL-3.0-only

package gen

import (
	"testing"

	"scampi.dev/scampi/lang/token"
)

func TestKeywordReplacementsCoverAllKeywords(t *testing.T) {
	for kw := range token.Keywords {
		if _, ok := keywordReplacements[kw]; !ok {
			t.Errorf("keyword %q has no entry in keywordReplacements", kw)
		}
	}
}

func TestKeywordReplacementsAreNotKeywords(t *testing.T) {
	for kw, replacement := range keywordReplacements {
		if _, ok := token.Keywords[replacement]; ok {
			t.Errorf("replacement for %q is itself a keyword: %q", kw, replacement)
		}
	}
}

func TestEscapeKeywordDigitPrefix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"6e_channel_size", "_6e_channel_size"},
		{"0rtt", "_0rtt"},
		{"name", "name"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := escapeKeyword(tt.in); got != tt.want {
			t.Errorf("escapeKeyword(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestToSnakeCaseHyphens(t *testing.T) {
	tests := []struct{ in, want string }{
		{"static-route_distance", "static_route_distance"},
		{"camelCase", "camel_case"},
		{"already_snake", "already_snake"},
		{"x-forwarded-for", "x_forwarded_for"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := toSnakeCase(tt.in); got != tt.want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
