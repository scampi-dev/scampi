// SPDX-License-Identifier: GPL-3.0-only

package mod_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/mod"
	"scampi.dev/scampi/source"
)

func TestComputeHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello")
	writeFile(t, dir, "b.txt", "world")

	h1, err := mod.ComputeHash(dir)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	h2, err := mod.ComputeHash(dir)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hashes differ: %q vs %q", h1, h2)
	}
}

func TestComputeHash_DifferentContent(t *testing.T) {
	dir1 := t.TempDir()
	writeFile(t, dir1, "a.txt", "hello")

	dir2 := t.TempDir()
	writeFile(t, dir2, "a.txt", "goodbye")

	h1, err := mod.ComputeHash(dir1)
	if err != nil {
		t.Fatalf("hash dir1: %v", err)
	}
	h2, err := mod.ComputeHash(dir2)
	if err != nil {
		t.Fatalf("hash dir2: %v", err)
	}
	if h1 == h2 {
		t.Error("expected different hashes for different content, got same")
	}
}

func TestComputeHash_IgnoresDotGit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello")

	h1, err := mod.ComputeHash(dir)
	if err != nil {
		t.Fatalf("hash without .git: %v", err)
	}

	// Add a .git directory with a file — hash must not change.
	gitDir := filepath.Join(dir, ".git")
	if mkErr := os.MkdirAll(gitDir, 0o755); mkErr != nil {
		t.Fatalf("mkdir .git: %v", mkErr)
	}
	writeFile(t, gitDir, "HEAD", "ref: refs/heads/main")

	h2, err := mod.ComputeHash(dir)
	if err != nil {
		t.Fatalf("hash with .git: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hash changed after adding .git: %q vs %q", h1, h2)
	}
}

func TestComputeHash_Format(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.scampi", "# scampi config")

	h, err := mod.ComputeHash(dir)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(h, "h1:") {
		t.Errorf("expected h1: prefix, got %q", h)
	}
	// "h1:" = 3 chars, sha256 hex = 64 chars => 67 total
	if len(h) != 67 {
		t.Errorf("expected length 67, got %d: %q", len(h), h)
	}
}

func TestWriteAndReadSum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scampi.sum")
	sums := map[string]string{
		"codeberg.org/foo/bar v1.0.0": "h1:abc123",
		"codeberg.org/baz/qux v2.1.0": "h1:def456",
	}

	if err := mod.WriteSum(context.Background(), source.LocalPosixSource{}, path, sums); err != nil {
		t.Fatalf("WriteSum: %v", err)
	}

	got, err := mod.ReadSum(context.Background(), source.LocalPosixSource{}, path)
	if err != nil {
		t.Fatalf("ReadSum: %v", err)
	}
	for k, v := range sums {
		if got[k] != v {
			t.Errorf("key %q: want %q, got %q", k, v, got[k])
		}
	}
	if len(got) != len(sums) {
		t.Errorf("want %d entries, got %d", len(sums), len(got))
	}
}

func TestReadSum_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scampi.sum")

	got, err := mod.ReadSum(context.Background(), source.LocalPosixSource{}, path)
	if err != nil {
		t.Fatalf("ReadSum on missing file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestWriteSum_Sorted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scampi.sum")
	sums := map[string]string{
		"codeberg.org/zzz/mod v1.0.0": "h1:zzz",
		"codeberg.org/aaa/mod v1.0.0": "h1:aaa",
		"codeberg.org/mmm/mod v1.0.0": "h1:mmm",
	}

	if err := mod.WriteSum(context.Background(), source.LocalPosixSource{}, path, sums); err != nil {
		t.Fatalf("WriteSum: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] < lines[i-1] {
			t.Errorf("lines not sorted at %d: %q comes before %q", i, lines[i-1], lines[i])
		}
	}
}

func TestVerifyModule_OK(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.scampi", "# scampi module")

	hash, err := mod.ComputeHash(dir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	dep := mod.Dependency{Path: "codeberg.org/foo/bar", Version: "v1.0.0"}
	sums := map[string]string{
		"codeberg.org/foo/bar v1.0.0": hash,
	}

	if err := mod.VerifyModule(nil, dep, dir, sums); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestVerifyModule_Mismatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.scampi", "# scampi module")

	dep := mod.Dependency{Path: "codeberg.org/foo/bar", Version: "v1.0.0"}
	sums := map[string]string{
		"codeberg.org/foo/bar v1.0.0": "h1:0000000000000000000000000000000000000000000000000000000000000000",
	}

	err := mod.VerifyModule(nil, dep, dir, sums)
	if err == nil {
		t.Fatal("expected error for mismatched hash, got nil")
	}
	var mismatch *mod.SumMismatchError
	if !isSumMismatch(err, &mismatch) {
		t.Errorf("expected SumMismatchError, got %T: %v", err, err)
	}
}

func TestVerifyModule_NotInSum(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.scampi", "# scampi module")

	dep := mod.Dependency{Path: "codeberg.org/new/module", Version: "v1.0.0"}
	sums := map[string]string{}

	if err := mod.VerifyModule(nil, dep, dir, sums); err != nil {
		t.Errorf("expected nil for new module, got %v", err)
	}
}

// Helpers
// -----------------------------------------------------------------------------

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
}

func isSumMismatch(err error, target **mod.SumMismatchError) bool {
	if m, ok := err.(*mod.SumMismatchError); ok {
		if target != nil {
			*target = m
		}
		return true
	}
	return false
}
