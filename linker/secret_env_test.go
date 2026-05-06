// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"testing"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/secret"
)

func mockEnv(env map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := env[name]
		return v, ok
	}
}

func TestSecretEnvBuiltin_ResolvesAndRegisters(t *testing.T) {
	r := secret.NewRedactor()
	fn := secretEnvBuiltin(
		mockEnv(map[string]string{"DB_PASSWORD": "test-fixture-password-1234"}),
		r,
	)

	v, errMsg := fn([]eval.Value{&eval.StringVal{V: "DB_PASSWORD"}}, nil)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	s, ok := v.(*eval.StringVal)
	if !ok {
		t.Fatalf("expected StringVal, got %T", v)
	}
	if s.V != "test-fixture-password-1234" {
		t.Errorf("V = %q, want test-fixture-password-1234", s.V)
	}
	if r.Size() != 1 {
		t.Errorf("redactor.Size = %d, want 1 (resolved value should register)", r.Size())
	}
	// Sanity: the redactor actually masks the value in output.
	const probe = "psql --password=test-fixture-password-1234"
	if got := r.Redact(probe); got == probe {
		t.Errorf("redactor failed to mask: %q", got)
	}
}

func TestSecretEnvBuiltin_DefaultDoesNotRegister(t *testing.T) {
	r := secret.NewRedactor()
	fn := secretEnvBuiltin(mockEnv(nil), r)

	v, errMsg := fn(
		[]eval.Value{
			&eval.StringVal{V: "MISSING_VAR"},
			&eval.StringVal{V: "fallback-default-value"},
		},
		nil,
	)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	s, ok := v.(*eval.StringVal)
	if !ok {
		t.Fatalf("expected StringVal, got %T", v)
	}
	if s.V != "fallback-default-value" {
		t.Errorf("V = %q, want fallback-default-value", s.V)
	}
	// Defaults are inline text in the config — not from a secret
	// store. Registering them would mask user-visible literals.
	if r.Size() != 0 {
		t.Errorf("redactor.Size = %d, want 0 (default must not register)", r.Size())
	}
}

func TestSecretEnvBuiltin_ErrorsOnMissingNoDefault(t *testing.T) {
	r := secret.NewRedactor()
	fn := secretEnvBuiltin(mockEnv(nil), r)

	_, errMsg := fn([]eval.Value{&eval.StringVal{V: "MISSING_VAR"}}, nil)
	if errMsg == "" {
		t.Fatal("expected error for missing env without default")
	}
}

func TestSecretEnvBuiltin_AcceptsKwargDefault(t *testing.T) {
	r := secret.NewRedactor()
	fn := secretEnvBuiltin(mockEnv(nil), r)

	v, errMsg := fn(
		[]eval.Value{&eval.StringVal{V: "MISSING_VAR"}},
		map[string]eval.Value{"default": &eval.StringVal{V: "kwarg-default"}},
	)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if v.(*eval.StringVal).V != "kwarg-default" {
		t.Errorf("V = %q, want kwarg-default", v.(*eval.StringVal).V)
	}
}

func TestSecretEnvBuiltin_NilRedactorIsNoOp(t *testing.T) {
	// LSP and similar paths may not have a redactor wired. The
	// builtin must still resolve the value cleanly — secrets just
	// won't be masked downstream (LSP doesn't render to terminal).
	fn := secretEnvBuiltin(mockEnv(map[string]string{"X": "y-actual-value"}), nil)
	v, errMsg := fn([]eval.Value{&eval.StringVal{V: "X"}}, nil)
	if errMsg != "" {
		t.Fatalf("unexpected error with nil redactor: %s", errMsg)
	}
	if v.(*eval.StringVal).V != "y-actual-value" {
		t.Errorf("V = %q, want y-actual-value", v.(*eval.StringVal).V)
	}
}
