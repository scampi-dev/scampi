// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/secret"
)

// std.read_file builtin
// -----------------------------------------------------------------------------

// readFileFromMap returns a fake reader backed by an in-memory map.
// Tests pin behavior without touching the real filesystem.
func readFileFromMap(files map[string][]byte) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		data, ok := files[path]
		if !ok {
			return nil, errors.New("file not found: " + path)
		}
		return data, nil
	}
}

func TestStdReadFile_TrimsTrailingNewline(t *testing.T) {
	fn := stdReadFileBuiltin(
		"/cfg",
		readFileFromMap(map[string][]byte{
			"/cfg/keys/host.pub": []byte("ssh-ed25519 AAAA hal9000\n"),
		}),
		false,
	)
	v, errMsg := fn([]eval.Value{&eval.StringVal{V: "keys/host.pub"}}, nil)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	s, ok := v.(*eval.StringVal)
	if !ok {
		t.Fatalf("expected StringVal, got %T", v)
	}
	if s.V != "ssh-ed25519 AAAA hal9000" {
		t.Errorf("V = %q, want %q (trailing newline must be trimmed)", s.V, "ssh-ed25519 AAAA hal9000")
	}
}

func TestStdReadFile_ResolvesAbsolutePath(t *testing.T) {
	fn := stdReadFileBuiltin(
		"/cfg",
		readFileFromMap(map[string][]byte{"/etc/hosts": []byte("127.0.0.1 localhost")}),
		false,
	)
	v, errMsg := fn([]eval.Value{&eval.StringVal{V: "/etc/hosts"}}, nil)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if v.(*eval.StringVal).V != "127.0.0.1 localhost" {
		t.Errorf("V = %q", v.(*eval.StringVal).V)
	}
}

func TestStdReadFile_ResolvesRelativePathAgainstConfigDir(t *testing.T) {
	fn := stdReadFileBuiltin(
		"/Users/me/skrynet",
		readFileFromMap(map[string][]byte{
			"/Users/me/skrynet/keys/host.pub": []byte("ssh-key"),
		}),
		false,
	)
	v, errMsg := fn([]eval.Value{&eval.StringVal{V: "keys/host.pub"}}, nil)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if v.(*eval.StringVal).V != "ssh-key" {
		t.Errorf("V = %q", v.(*eval.StringVal).V)
	}
}

func TestStdReadFile_ErrorsOnMissingPath(t *testing.T) {
	fn := stdReadFileBuiltin("/cfg", readFileFromMap(nil), false)
	_, errMsg := fn([]eval.Value{&eval.StringVal{V: "missing.txt"}}, nil)
	if errMsg == "" {
		t.Fatal("expected error for missing file")
	}
}

func TestStdReadFile_ErrorsOnEmptyPath(t *testing.T) {
	fn := stdReadFileBuiltin("/cfg", readFileFromMap(nil), false)
	_, errMsg := fn(nil, nil)
	if errMsg == "" {
		t.Fatal("expected error for empty path")
	}
}

func TestStdReadFile_LenientReturnsPlaceholderOnMissing(t *testing.T) {
	fn := stdReadFileBuiltin("/cfg", readFileFromMap(nil), true)
	v, errMsg := fn([]eval.Value{&eval.StringVal{V: "missing.txt"}}, nil)
	if errMsg != "" {
		t.Fatalf("lenient must not error: %s", errMsg)
	}
	s := v.(*eval.StringVal).V
	if s == "" {
		t.Error("placeholder should be non-empty")
	}
}

func TestStdReadFile_LenientReturnsPlaceholderOnEmptyPath(t *testing.T) {
	fn := stdReadFileBuiltin("/cfg", readFileFromMap(nil), true)
	v, errMsg := fn(nil, nil)
	if errMsg != "" {
		t.Fatalf("lenient must not error: %s", errMsg)
	}
	if v.(*eval.StringVal).V == "" {
		t.Error("placeholder expected")
	}
}

func TestStdReadFile_PreservesInteriorNewlines(t *testing.T) {
	body := "line one\nline two\nline three\n"
	fn := stdReadFileBuiltin(
		"/cfg",
		readFileFromMap(map[string][]byte{"/cfg/multi.txt": []byte(body)}),
		false,
	)
	v, _ := fn([]eval.Value{&eval.StringVal{V: "multi.txt"}}, nil)
	want := "line one\nline two\nline three"
	if got := v.(*eval.StringVal).V; got != want {
		t.Errorf("V = %q, want %q", got, want)
	}
}

// Live filesystem smoke test - confirms the builtin works end-to-end
// when wired with real os.ReadFile, not just the in-memory mock.
func TestStdReadFile_RealFilesystem(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "host.pub")
	if err := os.WriteFile(keyPath, []byte("real-key-from-disk\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	fn := stdReadFileBuiltin(dir, os.ReadFile, false)
	v, errMsg := fn([]eval.Value{&eval.StringVal{V: "host.pub"}}, nil)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if v.(*eval.StringVal).V != "real-key-from-disk" {
		t.Errorf("V = %q", v.(*eval.StringVal).V)
	}
}

// std.secret_env builtin
// -----------------------------------------------------------------------------

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
	// Defaults are inline text in the config - not from a secret
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
	// builtin must still resolve the value cleanly - secrets just
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
