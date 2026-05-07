// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"context"
	"encoding/json"
	"testing"

	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/spec"
)

func TestCompileRedact_FlatField(t *testing.T) {
	got, err := compileRedact([]string{"x_ssh_password"}, spec.SourceSpan{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].expr != ".x_ssh_password" {
		t.Errorf("expr = %q, want %q", got[0].expr, ".x_ssh_password")
	}
}

func TestCompileRedact_NestedAndIndexed(t *testing.T) {
	got, err := compileRedact([]string{"data.token", "items[0].secret", ".already_dotted"}, spec.SourceSpan{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	wants := []string{".data.token", ".items[0].secret", ".already_dotted"}
	if len(got) != len(wants) {
		t.Fatalf("len = %d, want %d", len(got), len(wants))
	}
	for i, w := range wants {
		if got[i].expr != w {
			t.Errorf("paths[%d] = %q, want %q", i, got[i].expr, w)
		}
	}
}

func TestCompileRedact_InvalidPath(t *testing.T) {
	_, err := compileRedact([]string{"data..password"}, spec.SourceSpan{})
	if err == nil {
		t.Fatalf("expected error for invalid path, got nil")
	}
	if _, ok := err.(RedactPathError); !ok {
		t.Fatalf("expected RedactPathError, got %T: %v", err, err)
	}
}

func TestCompileRedact_EmptySkipped(t *testing.T) {
	got, err := compileRedact([]string{"", "  ", "real_field"}, spec.SourceSpan{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

func TestApplyRedact_RegistersStringValue(t *testing.T) {
	r := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), r)

	body := mustParseJSON(t, `{"x_ssh_password": "F6rePKhg30!0", "auto_upgrade": false}`)
	redact, _ := compileRedact([]string{"x_ssh_password"}, spec.SourceSpan{})

	applyRedact(ctx, redact, body)

	if r.Size() != 1 {
		t.Fatalf("size = %d, want 1", r.Size())
	}
	got := r.Redact("the password is F6rePKhg30!0 today")
	want := "the password is ***SECRET*** today"
	if got != want {
		t.Errorf("redact = %q, want %q", got, want)
	}
}

func TestApplyRedact_NestedPath(t *testing.T) {
	r := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), r)

	body := mustParseJSON(t, `{"data": {"creds": {"token": "supersecrettoken123"}}}`)
	redact, _ := compileRedact([]string{"data.creds.token"}, spec.SourceSpan{})

	applyRedact(ctx, redact, body)

	if r.Size() != 1 {
		t.Fatalf("size = %d, want 1", r.Size())
	}
	if r.Redact("supersecrettoken123") != "***SECRET***" {
		t.Errorf("nested value not redacted")
	}
}

func TestApplyRedact_ArrayIndex(t *testing.T) {
	r := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), r)

	body := mustParseJSON(t, `{"keys": ["alpha-secret-12345", "beta-leak-67890"]}`)
	redact, _ := compileRedact([]string{"keys[0]"}, spec.SourceSpan{})

	applyRedact(ctx, redact, body)

	if r.Redact("alpha-secret-12345") != "***SECRET***" {
		t.Errorf("indexed value not redacted")
	}
	// keys[1] not in the redact list — must NOT be redacted.
	if r.Redact("beta-leak-67890") != "beta-leak-67890" {
		t.Errorf("non-redacted value got redacted")
	}
}

func TestApplyRedact_MultiplePaths(t *testing.T) {
	r := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), r)

	body := mustParseJSON(t, `{
    "x_ssh_password": "ssh_pw_value_long",
    "x_ssh_sha512passwd": "$6$abcdef$hashedhashlonghash",
    "x_api_token": "tok_alsosecret_999"
  }`)
	redact, _ := compileRedact([]string{"x_ssh_password", "x_ssh_sha512passwd", "x_api_token"}, spec.SourceSpan{})

	applyRedact(ctx, redact, body)

	if r.Size() != 3 {
		t.Fatalf("size = %d, want 3", r.Size())
	}
}

func TestApplyRedact_NoRedactorOnContext_Noop(t *testing.T) {
	body := mustParseJSON(t, `{"password": "leaked"}`)
	redact, _ := compileRedact([]string{"password"}, spec.SourceSpan{})

	// No panic, no nothing — just returns.
	applyRedact(context.Background(), redact, body)
}

func TestApplyRedact_PathDoesNotMatch(t *testing.T) {
	r := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), r)

	body := mustParseJSON(t, `{"unrelated": "field"}`)
	redact, _ := compileRedact([]string{"x_ssh_password", "data.token"}, spec.SourceSpan{})

	applyRedact(ctx, redact, body)

	if r.Size() != 0 {
		t.Errorf("size = %d, want 0 — paths shouldn't match", r.Size())
	}
}

func TestRegisterRedact_ParsesBytes(t *testing.T) {
	r := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), r)

	redact, _ := compileRedact([]string{"password"}, spec.SourceSpan{})
	registerRedact(ctx, redact, []byte(`{"password": "hunter2-the-secret"}`))

	if r.Redact("hunter2-the-secret") != "***SECRET***" {
		t.Errorf("not redacted via registerRedact bytes wrapper")
	}
}

func TestRegisterRedact_NonJSONBody_Skipped(t *testing.T) {
	r := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), r)

	redact, _ := compileRedact([]string{"password"}, spec.SourceSpan{})
	// Non-JSON body — silently skipped, no error.
	registerRedact(ctx, redact, []byte(`<html>not json</html>`))

	if r.Size() != 0 {
		t.Errorf("size = %d, want 0", r.Size())
	}
}

func TestApplyRedact_NumberAndBoolCoerced(t *testing.T) {
	r := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), r)

	body := mustParseJSON(t, `{"port": 8443, "debug": true}`)
	redact, _ := compileRedact([]string{"port", "debug"}, spec.SourceSpan{})

	applyRedact(ctx, redact, body)

	// Numbers and bools coerce via fmt.Sprintf("%v", ...). The
	// minRedactLen guard (4) drops "true" exactly at the boundary.
	if r.Redact("8443") != "***SECRET***" {
		t.Errorf("numeric value not redacted")
	}
}

func mustParseJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}
