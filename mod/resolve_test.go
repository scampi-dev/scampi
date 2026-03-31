// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/source"
)

var (
	testCtx = context.Background()
	testSrc = source.LocalPosixSource{}
)

func makeModule(t *testing.T, deps ...Dependency) *Module {
	t.Helper()
	return &Module{
		Module:   "codeberg.org/test/project",
		Filename: "scampi.mod",
		Require:  deps,
	}
}

func dep(path, version string, line int) Dependency {
	return Dependency{Path: path, Version: version, Line: line}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("# stub"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestResolve_IndexEntry(t *testing.T) {
	cache := t.TempDir()
	modDir := filepath.Join(cache, "codeberg.org/user/npm@v1.0.0")
	writeFile(t, filepath.Join(modDir, "_index.scampi"))

	m := makeModule(t, dep("codeberg.org/user/npm", "v1.0.0", 2))
	got, err := Resolve(testCtx, testSrc, m, "codeberg.org/user/npm", cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(modDir, "_index.scampi")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolve_NameEntry(t *testing.T) {
	cache := t.TempDir()
	modDir := filepath.Join(cache, "codeberg.org/user/npm@v1.0.0")
	writeFile(t, filepath.Join(modDir, "npm.scampi"))

	m := makeModule(t, dep("codeberg.org/user/npm", "v1.0.0", 2))
	got, err := Resolve(testCtx, testSrc, m, "codeberg.org/user/npm", cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(modDir, "npm.scampi")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolve_IndexTakesPrecedenceOverName(t *testing.T) {
	cache := t.TempDir()
	modDir := filepath.Join(cache, "codeberg.org/user/npm@v1.0.0")
	writeFile(t, filepath.Join(modDir, "_index.scampi"))
	writeFile(t, filepath.Join(modDir, "npm.scampi"))

	m := makeModule(t, dep("codeberg.org/user/npm", "v1.0.0", 2))
	got, err := Resolve(testCtx, testSrc, m, "codeberg.org/user/npm", cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(modDir, "_index.scampi")
	if got != want {
		t.Errorf("_index.scampi should take precedence: got %s, want %s", got, want)
	}
}

func TestResolve_Subpath(t *testing.T) {
	cache := t.TempDir()
	modDir := filepath.Join(cache, "codeberg.org/user/npm@v1.0.0")
	writeFile(t, filepath.Join(modDir, "internal", "helpers.scampi"))

	m := makeModule(t, dep("codeberg.org/user/npm", "v1.0.0", 2))
	got, err := Resolve(testCtx, testSrc, m, "codeberg.org/user/npm/internal/helpers", cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(modDir, "internal", "helpers.scampi")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolve_SubpathIndex(t *testing.T) {
	cache := t.TempDir()
	modDir := filepath.Join(cache, "codeberg.org/user/npm@v1.0.0")
	writeFile(t, filepath.Join(modDir, "internal", "_index.scampi"))

	m := makeModule(t, dep("codeberg.org/user/npm", "v1.0.0", 2))
	got, err := Resolve(testCtx, testSrc, m, "codeberg.org/user/npm/internal", cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(modDir, "internal", "_index.scampi")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolve_NotInRequireTable(t *testing.T) {
	cache := t.TempDir()
	m := makeModule(t)
	_, err := Resolve(testCtx, testSrc, m, "codeberg.org/user/npm", cache)
	if err == nil {
		t.Fatal("expected error")
	}
	var notFound *ModuleNotFoundError
	if !asError(err, &notFound) {
		t.Fatalf("expected *ModuleNotFoundError, got %T: %v", err, err)
	}
	if notFound.LoadPath != "codeberg.org/user/npm" {
		t.Errorf("LoadPath = %q, want %q", notFound.LoadPath, "codeberg.org/user/npm")
	}
}

func TestResolve_NotCached(t *testing.T) {
	cache := t.TempDir()
	m := makeModule(t, dep("codeberg.org/user/npm", "v1.0.0", 3))
	_, err := Resolve(testCtx, testSrc, m, "codeberg.org/user/npm", cache)
	if err == nil {
		t.Fatal("expected error")
	}
	var notCached *ModuleNotCachedError
	if !asError(err, &notCached) {
		t.Fatalf("expected *ModuleNotCachedError, got %T: %v", err, err)
	}
	if notCached.ModPath != "codeberg.org/user/npm" {
		t.Errorf("ModPath = %q", notCached.ModPath)
	}
	if notCached.Version != "v1.0.0" {
		t.Errorf("Version = %q", notCached.Version)
	}
	if notCached.Source.StartLine != 3 {
		t.Errorf("Source.StartLine = %d, want 3", notCached.Source.StartLine)
	}
}

func TestResolve_NoEntryPoint(t *testing.T) {
	cache := t.TempDir()
	modDir := filepath.Join(cache, "codeberg.org/user/npm@v1.0.0")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}

	m := makeModule(t, dep("codeberg.org/user/npm", "v1.0.0", 2))
	_, err := Resolve(testCtx, testSrc, m, "codeberg.org/user/npm", cache)
	if err == nil {
		t.Fatal("expected error")
	}
	var noEntry *ModuleNoEntryPointError
	if !asError(err, &noEntry) {
		t.Fatalf("expected *ModuleNoEntryPointError, got %T: %v", err, err)
	}
	if noEntry.ModPath != "codeberg.org/user/npm" {
		t.Errorf("ModPath = %q", noEntry.ModPath)
	}
	if len(noEntry.Tried) == 0 {
		t.Error("Tried should not be empty")
	}
}

func TestDefaultCacheDir_XDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	got := DefaultCacheDir()
	want := "/tmp/xdg/scampi/mod"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDefaultCacheDir_Home(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	got := DefaultCacheDir()
	if got == "" {
		t.Error("DefaultCacheDir should not be empty")
	}
	if !filepath.IsAbs(got) {
		t.Errorf("DefaultCacheDir should be absolute: %s", got)
	}
}

// asError is a type-assertion helper for pointer error types.
func asError[T any](err error, target *T) bool {
	if v, ok := err.(T); ok {
		*target = v
		return true
	}
	return false
}
