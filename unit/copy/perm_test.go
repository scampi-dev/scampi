package copy

import (
	"strings"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/spec"
)

func TestParsePerm_InvalidPermissions(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid octal digit",
			input: "062i",
		},
		{
			name:  "octal out of range",
			input: "9120",
		},
		{
			name:  "ls too short",
			input: "rw-rw",
		},
		{
			name:  "garbage",
			input: "wat",
		},
	}

	wantHelpSub := []string{
		"octal",
		"ls",
		"posix",
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePerm(tc.input, spec.SourceSpan{})
			if err == nil {
				t.Fatalf("expected error for input %q", tc.input)
			}

			dp, ok := err.(diagnostic.DiagnosticProvider)
			if !ok {
				t.Fatalf("error does not expose diagnostics: %T", err)
			}

			evs := dp.Diagnostics(event.Subject{})
			if len(evs) != 1 {
				t.Fatalf("expected 1 diagnostic, got %d", len(evs))
			}

			detail, ok := evs[0].Detail.(event.DiagnosticDetail)
			if !ok {
				t.Fatalf("expected DiagnosticDetail, got %T", evs[0].Detail)
			}

			text := strings.ToLower(detail.Template.Text)
			if !strings.Contains(text, "permission") {
				t.Fatalf("expected diagnostic text to mention permission, got %q", text)
			}

			help := strings.ToLower(detail.Template.Help)
			for _, sub := range wantHelpSub {
				if !strings.Contains(help, sub) {
					t.Fatalf("expected help to mention %q, got %q", sub, help)
				}
			}
		})
	}
}

func TestParsePerm_ValidPermissions(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		// ---- octal ----
		{"octal lowest", "0000"},
		{"octal typical", "0644"},
		{"octal highest", "0777"},

		// ---- ls-style ----
		{"ls none", "---------"},
		{"ls typical", "rw-r--r--"},
		{"ls full", "rwxrwxrwx"},

		// ---- posix ----
		{"posix empty", "u=,g=,o="},
		{"posix typical", "u=rw,g=r,o=r"},
		{"posix full", "u=rwx,g=rwx,o=rwx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePerm(tc.input, spec.SourceSpan{})
			if err != nil {
				t.Fatalf("expected success for %q, got %v", tc.input, err)
			}
		})
	}
}
