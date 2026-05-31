// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"scampi.dev/scampi/internal/engine"
)

type nopLog struct{}

func (nopLog) Debug(context.Context, string, ...any) {}
func (nopLog) Info(context.Context, string, ...any)  {}
func (nopLog) Warn(context.Context, string, ...any)  {}
func (nopLog) Error(context.Context, string, ...any) {}

// Golden
// -----------------------------------------------------------------------------

type goldenExpected struct {
	Error  string
	Files  map[string]string
	Dirs   []string
	Absent []string
}

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
	// {{TMP}} subs in both inputs and expected so ref-derived content
	// can name absolute tempdir paths.
	expectedYAML = []byte(strings.ReplaceAll(string(expectedYAML), "{{TMP}}", target))
	var want goldenExpected
	if yerr := yaml.Unmarshal(expectedYAML, &want); yerr != nil {
		t.Fatalf("expected.yaml: %v", yerr)
	}

	gotErr := engine.Apply(t.Context(), cfg, nopLog{})

	switch want.Error {
	case "":
		if gotErr != nil {
			t.Fatalf("expected success, got: %v", gotErr)
		}
	case "snapshot":
		if !errors.Is(gotErr, engine.ErrSnapshotRejected) {
			t.Fatalf("expected engine.ErrSnapshotRejected, got: %v", gotErr)
		}
	case "apply":
		if !errors.Is(gotErr, engine.ErrApplyFailed) {
			t.Fatalf("expected engine.ErrApplyFailed, got: %v", gotErr)
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

// In-sync skip can't be asserted by post-state alone; the observable
// is mtime not advancing.
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
	if err := engine.Apply(t.Context(), cfg, nopLog{}); err != nil {
		t.Fatalf("Apply: %v", err)
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

func writeConfig(t *testing.T, hcl string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.hcl"), []byte(hcl), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Run mode
// -----------------------------------------------------------------------------

// 24h interval + short ctx deadline proves the initial reconcile
// fires before the first ticker wait.
func TestRun_AppliesOnceAtStart(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	cfg := writeConfig(t, `
file "x" {
  path    = "`+target+`"
  content = "hi"
}
`)
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	if err := engine.Run(ctx, cfg, 24*time.Hour, nopLog{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hi" {
		t.Errorf("content = %q, want %q", got, "hi")
	}
}

func TestRun_PicksUpChanges(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	cfg := t.TempDir()
	hclPath := filepath.Join(cfg, "main.hcl")
	writeHCL := func(content string) {
		t.Helper()
		hcl := `
file "x" {
  path    = "` + target + `"
  content = "` + content + `"
}
`
		if err := os.WriteFile(hclPath, []byte(hcl), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeHCL("first")

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- engine.Run(ctx, cfg, 20*time.Millisecond, nopLog{})
	}()

	waitForFile(t, target, []byte("first"), time.Second)

	writeHCL("second")
	waitForFile(t, target, []byte("second"), time.Second)

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func waitForFile(t *testing.T, path string, want []byte, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got, err := os.ReadFile(path)
		if err == nil && bytes.Equal(got, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := os.ReadFile(path)
	t.Fatalf("%s never became %q (last saw %q)", path, want, got)
}
