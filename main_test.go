// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type nopLog struct{}

func (nopLog) Debug(context.Context, string, ...any) {}
func (nopLog) Info(context.Context, string, ...any)  {}
func (nopLog) Warn(context.Context, string, ...any)  {}
func (nopLog) Error(context.Context, string, ...any) {}

// Golden
// -----------------------------------------------------------------------------

// goldenExpected is the per-case expectation. Error categorizes the
// run's return: "" success, "snapshot" rejected, "apply" runtime fail.
// Files / Dirs / Absent assert post-state of the target tempdir
// (relative paths).
type goldenExpected struct {
	Error  string
	Files  map[string]string
	Dirs   []string
	Absent []string
}

// TestGolden walks testdata/golden/*. Each case is a dir containing
// any number of *.hcl source files and one expected.yaml. The token
// {{TMP}} in *.hcl is substituted with a per-test tempdir before apply.
func TestGolden(t *testing.T) {
	cases, err := filepath.Glob("testdata/golden/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no golden cases under testdata/golden/")
	}
	for _, dir := range cases {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			runGoldenCase(t, dir)
		})
	}
}

func runGoldenCase(t *testing.T, caseDir string) {
	t.Helper()
	target := t.TempDir()
	cfg := t.TempDir()

	inputs, err := filepath.Glob(filepath.Join(caseDir, "*.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no *.hcl inputs under %s", caseDir)
	}
	for _, in := range inputs {
		src, rerr := os.ReadFile(in)
		if rerr != nil {
			t.Fatal(rerr)
		}
		out := strings.ReplaceAll(string(src), "{{TMP}}", target)
		dst := filepath.Join(cfg, filepath.Base(in))
		if werr := os.WriteFile(dst, []byte(out), 0o644); werr != nil {
			t.Fatal(werr)
		}
	}

	expectedYAML, err := os.ReadFile(filepath.Join(caseDir, "expected.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var want goldenExpected
	if yerr := yaml.Unmarshal(expectedYAML, &want); yerr != nil {
		t.Fatalf("expected.yaml: %v", yerr)
	}

	gotErr := run(t.Context(), []string{"apply", cfg}, nopLog{})

	switch want.Error {
	case "":
		if gotErr != nil {
			t.Fatalf("expected success, got: %v", gotErr)
		}
	case "snapshot":
		if !errors.Is(gotErr, ErrSnapshotRejected) {
			t.Fatalf("expected ErrSnapshotRejected, got: %v", gotErr)
		}
	case "apply":
		if !errors.Is(gotErr, ErrApplyFailed) {
			t.Fatalf("expected ErrApplyFailed, got: %v", gotErr)
		}
	default:
		t.Fatalf("unknown expected.error %q", want.Error)
	}

	for relPath, wantContent := range want.Files {
		got, rerr := os.ReadFile(filepath.Join(target, relPath))
		if rerr != nil {
			t.Errorf("file %s: %v", relPath, rerr)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("file %s: content = %q, want %q", relPath, got, wantContent)
		}
	}
	for _, relPath := range want.Dirs {
		info, serr := os.Stat(filepath.Join(target, relPath))
		if serr != nil {
			t.Errorf("dir %s: %v", relPath, serr)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s: not a directory", relPath)
		}
	}
	for _, relPath := range want.Absent {
		if _, serr := os.Stat(filepath.Join(target, relPath)); !errors.Is(serr, os.ErrNotExist) {
			t.Errorf("expected absent: %s exists", relPath)
		}
	}
}

// Non-golden
// -----------------------------------------------------------------------------

// In-sync skip can't be asserted by post-state alone (the file is
// present with the same content before and after either way); the
// observable is mtime not advancing.
func TestApply_FileInSyncSkipsWrite(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, `
file "etc" {
  path    = "`+target+`"
  content = "hello"
}
`)
	if err := run(t.Context(), []string{"apply", cfg}, nopLog{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("mtime changed from %v to %v; expected no write when in sync",
			before.ModTime(), after.ModTime())
	}
}

func TestRun_NoArgsReturnsUsage(t *testing.T) {
	err := run(t.Context(), nil, nopLog{})
	if err == nil {
		t.Fatal("expected usage error with no args")
	}
	if errors.Is(err, ErrSnapshotRejected) || errors.Is(err, ErrApplyFailed) {
		t.Errorf("usage error should not be in the apply taxonomy, got: %v", err)
	}
}

func writeConfig(t *testing.T, hcl string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.hcl"), []byte(hcl), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
