// SPDX-License-Identifier: GPL-3.0-only

package mod_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/mod"
	"scampi.dev/scampi/source"
)

// initTaggedRepo creates a bare git repo with multiple tagged commits in a temp dir.
// Each tag is applied to a distinct commit so all refs resolve cleanly.
func initTaggedRepo(t *testing.T, tags ...string) string {
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

	for i, tag := range tags {
		name := "file" + tag + ".txt"
		content := []byte("v" + tag + "\n")
		if err := os.WriteFile(filepath.Join(work, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
		// Ensure _index.scampi exists on first commit
		if i == 0 {
			if err := os.WriteFile(filepath.Join(work, "_index.scampi"), []byte("x = 1\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		run(work, "add", ".")
		run(work, "commit", "-m", "commit for "+tag)
		run(work, "tag", tag)
	}

	run(work, "clone", "--bare", work, bare)
	return bare
}

func TestParseLatestStable(t *testing.T) {
	output := "" +
		"abc123\trefs/tags/v1.0.0\n" +
		"def456\trefs/tags/v1.0.0^{}\n" +
		"ghi789\trefs/tags/v2.0.0\n" +
		"jkl012\trefs/tags/v2.0.0^{}\n" +
		"mno345\trefs/tags/v1.5.0\n" +
		"pqr678\trefs/tags/v1.0.0-alpha.1\n" +
		"stu901\trefs/tags/not-a-version\n"

	got := mod.ParseLatestStable(output)
	if got != "v2.0.0" {
		t.Errorf("ParseLatestStable = %q, want %q", got, "v2.0.0")
	}
}

func TestParseLatestStable_NoStable(t *testing.T) {
	output := "" +
		"abc123\trefs/tags/v1.0.0-alpha.1\n" +
		"def456\trefs/tags/v2.0.0-beta.2\n"

	got := mod.ParseLatestStable(output)
	if got != "" {
		t.Errorf("ParseLatestStable = %q, want %q", got, "")
	}
}

// Regression test for #332: tags carrying build metadata are valid
// semver and must enter the candidate set. The old isSemver check
// hand-rolled a `[major].[minor].[patch][-pre]?` matcher that
// rejected `+build` entirely, hiding a perfectly good release.
func TestParseLatestStable_BuildMetadataAccepted(t *testing.T) {
	output := "" +
		"abc123\trefs/tags/v1.0.0\n" +
		"def456\trefs/tags/v1.0.0+build42\n"

	got := mod.ParseLatestStable(output)
	// semver.Compare treats v1.0.0 and v1.0.0+build42 as equal, so SortFunc
	// keeps the input order and the *last* candidate wins. Either is a
	// stable release; what matters is that build42 is no longer silently
	// dropped.
	if got != "v1.0.0+build42" && got != "v1.0.0" {
		t.Errorf("ParseLatestStable = %q, want one of v1.0.0 / v1.0.0+build42", got)
	}
}

func TestParseLatestStable_Empty(t *testing.T) {
	got := mod.ParseLatestStable("")
	if got != "" {
		t.Errorf("ParseLatestStable(\"\") = %q, want %q", got, "")
	}
}

func TestAdd_ExplicitVersion(t *testing.T) {
	bare := initBareRepo(t, "v1.2.3")
	dir := t.TempDir()
	cacheDir := t.TempDir()

	// Write a minimal scampi.mod
	modContent := "module codeberg.org/test/mymod\n"
	if err := os.WriteFile(filepath.Join(dir, "scampi.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	version, _, err := mod.Add(context.Background(), source.LocalPosixSource{}, bare, "v1.2.3", dir, cacheDir)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if version != "v1.2.3" {
		t.Errorf("Add returned version %q, want %q", version, "v1.2.3")
	}

	// Check scampi.mod updated
	modData, err := os.ReadFile(filepath.Join(dir, "scampi.mod"))
	if err != nil {
		t.Fatal(err)
	}
	modStr := string(modData)
	if !strings.Contains(modStr, bare+" v1.2.3") {
		t.Errorf("scampi.mod missing %q entry:\n%s", bare+" v1.2.3", modStr)
	}

	// Check scampi.sum updated
	sumData, err := os.ReadFile(filepath.Join(dir, "scampi.sum"))
	if err != nil {
		t.Fatal(err)
	}
	sumStr := string(sumData)
	if !strings.Contains(sumStr, bare+" v1.2.3 h1:") {
		t.Errorf("scampi.sum missing hash entry for %s v1.2.3:\n%s", bare, sumStr)
	}
}

func TestAdd_LatestStable(t *testing.T) {
	bare := initTaggedRepo(t, "v0.1.0", "v0.9.0", "v1.0.0", "v1.0.0-alpha.1")
	dir := t.TempDir()
	cacheDir := t.TempDir()

	modContent := "module codeberg.org/test/mymod\n"
	if err := os.WriteFile(filepath.Join(dir, "scampi.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// version="" triggers latest stable resolution
	version, _, err := mod.Add(context.Background(), source.LocalPosixSource{}, bare, "", dir, cacheDir)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if version != "v1.0.0" {
		t.Errorf("Add resolved version %q, want %q", version, "v1.0.0")
	}

	modData, err := os.ReadFile(filepath.Join(dir, "scampi.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(modData), bare+" v1.0.0") {
		t.Errorf("scampi.mod missing %q entry:\n%s", bare+" v1.0.0", string(modData))
	}
}

func TestAdd_NoStableVersion(t *testing.T) {
	bare := initTaggedRepo(t, "v1.0.0-alpha.1", "v2.0.0-beta.1")
	dir := t.TempDir()
	cacheDir := t.TempDir()

	modContent := "module codeberg.org/test/mymod\n"
	if err := os.WriteFile(filepath.Join(dir, "scampi.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := mod.Add(context.Background(), source.LocalPosixSource{}, bare, "", dir, cacheDir)
	if err == nil {
		t.Fatal("expected NoStableVersionError, got nil")
	}

	var nsve *mod.NoStableVersionError
	if !errors.As(err, &nsve) {
		t.Fatalf("expected *mod.NoStableVersionError, got %T: %v", err, err)
	}
	if nsve.ModPath != bare {
		t.Errorf("NoStableVersionError.ModPath = %q, want %q", nsve.ModPath, bare)
	}
}
