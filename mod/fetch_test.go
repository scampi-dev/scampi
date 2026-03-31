// SPDX-License-Identifier: GPL-3.0-only

package mod_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/mod"
)

// initBareRepo creates a bare git repo with one tagged commit in a temp dir.
func initBareRepo(t *testing.T, tag string) string {
	t.Helper()
	work := t.TempDir()
	bare := filepath.Join(t.TempDir(), "repo.git")

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	run(work, "init")
	run(work, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "_index.scampi"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(work, "add", ".")
	run(work, "commit", "-m", "initial")
	run(work, "tag", tag)
	run(work, "clone", "--bare", work, bare)

	return bare
}

func TestFetch_Basic(t *testing.T) {
	bare := initBareRepo(t, "v0.1.0")
	cacheDir := t.TempDir()

	dep := mod.Dependency{Path: bare, Version: "v0.1.0"}
	if err := mod.Fetch(dep, cacheDir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	dest := filepath.Join(cacheDir, bare+"@v0.1.0")

	// Entry point file must exist.
	if _, err := os.Stat(filepath.Join(dest, "_index.scampi")); err != nil {
		t.Errorf("_index.scampi not found after fetch: %v", err)
	}

	// .git must be removed.
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		t.Error(".git directory was not removed after fetch")
	}
}

func TestFetch_Idempotent(t *testing.T) {
	bare := initBareRepo(t, "v0.1.0")
	cacheDir := t.TempDir()

	dep := mod.Dependency{Path: bare, Version: "v0.1.0"}
	if err := mod.Fetch(dep, cacheDir); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	if err := mod.Fetch(dep, cacheDir); err != nil {
		t.Fatalf("second Fetch (idempotent): %v", err)
	}
}

func TestFetch_BadTag(t *testing.T) {
	bare := initBareRepo(t, "v0.1.0")
	cacheDir := t.TempDir()

	dep := mod.Dependency{Path: bare, Version: "v9.9.9"}
	err := mod.Fetch(dep, cacheDir)
	if err == nil {
		t.Fatal("expected error for nonexistent tag, got nil")
	}

	var fe *mod.FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *mod.FetchError, got %T: %v", err, err)
	}
	if fe.ModPath != bare {
		t.Errorf("FetchError.ModPath = %q, want %q", fe.ModPath, bare)
	}
	if fe.Version != "v9.9.9" {
		t.Errorf("FetchError.Version = %q, want %q", fe.Version, "v9.9.9")
	}
}
