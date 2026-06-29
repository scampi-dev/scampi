// SPDX-License-Identifier: GPL-3.0-only

package escalate

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/target"
)

// testRunner implements target.Command by shelling out locally.
type testRunner struct{}

func (testRunner) RunCommand(_ context.Context, cmd string) (target.CommandResult, error) {
	c := exec.Command("sh", "-c", cmd)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return target.CommandResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitErr.ExitCode(),
			}, nil
		}
		return target.CommandResult{}, err
	}
	return target.CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func (testRunner) RunPrivileged(ctx context.Context, cmd string) (target.CommandResult, error) {
	return testRunner{}.RunCommand(ctx, cmd)
}

// newStatScript creates a fake escalation script that logs args and
// echoes the given output. Returns the script path and a log reader.
func newStatScript(t *testing.T, output string) (string, func() string) {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "cmd.log")
	script := filepath.Join(dir, "escalate")

	err := os.WriteFile(script, []byte(
		"#!/bin/sh\necho \"$*\" >> "+logFile+"\necho '"+output+"'\n",
	), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	readLog := func() string {
		data, _ := os.ReadFile(logFile)
		return strings.TrimSpace(string(data))
	}
	return script, readLog
}

// GNU stat command construction
// -----------------------------------------------------------------------------

func TestGNUStat_Command(t *testing.T) {
	script, readLog := newStatScript(t, "81a4 1024 1710756000 config")
	info, err := GNUStat(t.Context(), testRunner{}, script, "/etc/config", true)
	if err != nil {
		t.Fatal(err)
	}
	got := readLog()
	if !strings.HasPrefix(got, "stat -L -c") || !strings.HasSuffix(got, "/etc/config") {
		t.Fatalf("unexpected command: %q", got)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("expected 0644, got %s", info.Mode().Perm())
	}
	if info.Size() != 1024 {
		t.Fatalf("expected size 1024, got %d", info.Size())
	}
}

func TestGNULstat_Command(t *testing.T) {
	script, readLog := newStatScript(t, "81a4 0 1710756000 link")
	_, err := GNUStat(t.Context(), testRunner{}, script, "/etc/link", false)
	if err != nil {
		t.Fatal(err)
	}
	got := readLog()
	if strings.Contains(got, "-L") {
		t.Fatalf("lstat should not use -L: %q", got)
	}
	if !strings.HasPrefix(got, "stat -c") || !strings.HasSuffix(got, "/etc/link") {
		t.Fatalf("unexpected command: %q", got)
	}
}

func TestGNUGetOwner_Command(t *testing.T) {
	script, readLog := newStatScript(t, "root wheel")
	owner, err := GNUGetOwner(t.Context(), testRunner{}, script, "/etc/config")
	if err != nil {
		t.Fatal(err)
	}
	got := readLog()
	if !strings.HasPrefix(got, "stat -L -c") || !strings.HasSuffix(got, "/etc/config") {
		t.Fatalf("unexpected command: %q", got)
	}
	if owner.User != "root" || owner.Group != "wheel" {
		t.Fatalf("expected root:wheel, got %s:%s", owner.User, owner.Group)
	}
}

// BSD stat command construction
// -----------------------------------------------------------------------------

func TestBSDStat_Command(t *testing.T) {
	script, readLog := newStatScript(t, "81a4 512 1710756000 hosts")
	info, err := BSDStat(t.Context(), testRunner{}, script, "/etc/hosts", true)
	if err != nil {
		t.Fatal(err)
	}
	got := readLog()
	if !strings.HasPrefix(got, "stat -L -f") || !strings.HasSuffix(got, "/etc/hosts") {
		t.Fatalf("unexpected command: %q", got)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("expected 0644, got %s", info.Mode().Perm())
	}
	if info.Size() != 512 {
		t.Fatalf("expected size 512, got %d", info.Size())
	}
}

func TestBSDGetOwner_Command(t *testing.T) {
	script, readLog := newStatScript(t, "root staff")
	owner, err := BSDGetOwner(t.Context(), testRunner{}, script, "/etc/hosts")
	if err != nil {
		t.Fatal(err)
	}
	got := readLog()
	if !strings.HasPrefix(got, "stat -L -f") || !strings.HasSuffix(got, "/etc/hosts") {
		t.Fatalf("unexpected command: %q", got)
	}
	if owner.User != "root" || owner.Group != "staff" {
		t.Fatalf("expected root:staff, got %s:%s", owner.User, owner.Group)
	}
}

// Stat output parsing
// -----------------------------------------------------------------------------

func TestParseStatOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantMode fs.FileMode
		wantSize int64
		wantDir  bool
	}{
		{"regular file 644", "81a4 512 1710756000 foo", 0o644, 512, false},
		{"regular file 755", "81ed 0 1710756000 bar", 0o755, 0, false},
		{"directory 755", "41ed 4096 1710756000 mydir", fs.ModeDir | 0o755, 4096, true},
		{"symlink", "a1ff 10 1710756000 link", fs.ModeSymlink | 0o777, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseStatOutput(tt.output, "/test/"+tt.name)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode() != tt.wantMode {
				t.Errorf("mode: got %s, want %s", info.Mode(), tt.wantMode)
			}
			if info.Size() != tt.wantSize {
				t.Errorf("size: got %d, want %d", info.Size(), tt.wantSize)
			}
			if info.IsDir() != tt.wantDir {
				t.Errorf("isDir: got %v, want %v", info.IsDir(), tt.wantDir)
			}
		})
	}
}
