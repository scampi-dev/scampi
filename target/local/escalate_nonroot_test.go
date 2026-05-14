// SPDX-License-Identifier: GPL-3.0-only

//go:build !runasroot

// Tests that exercise the "real user lacks permission, escalate"
// fallback paths. They depend on chmod 0o000 actually denying access,
// which only holds for non-root users. Pass `-tags runasroot` to skip
// the whole file when running the suite as root.

package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/posix"
)

func TestDetectEscalation(t *testing.T) {
	var tgt POSIXTarget
	tgt.Runner = tgt.RunCommand
	result := posix.DetectEscalation(context.Background(), tgt.RunCommand, false)
	switch result {
	case "sudo", "doas", "":
	default:
		t.Fatalf("unexpected escalation tool: %q", result)
	}
}

func TestStat_FallsBackOnPermission(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "restricted")
	if err := os.Mkdir(inner, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(inner, 0o755) }()

	path := filepath.Join(inner, "file")
	tgt, readLog := newStatTarget(t, "81a4 42 1710756000 file")
	info, err := tgt.Stat(context.Background(), path)
	if err != nil {
		t.Fatalf("expected fallback to handle it, got: %v", err)
	}

	cmd := readLog()
	if !strings.Contains(cmd, "stat -L") {
		t.Fatalf("expected escalated stat -L, got: %q", cmd)
	}
	if info.Size() != 42 {
		t.Fatalf("expected size 42, got %d", info.Size())
	}
}

func TestGetOwner_FallsBackOnPermission(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "restricted")
	if err := os.Mkdir(inner, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(inner, 0o755) }()

	path := filepath.Join(inner, "file")
	tgt, _ := newStatTarget(t, "app app")
	owner, err := tgt.GetOwner(context.Background(), path)
	if err != nil {
		t.Fatalf("expected fallback to handle it, got: %v", err)
	}
	if owner.User != "app" || owner.Group != "app" {
		t.Fatalf("expected app:app, got %s:%s", owner.User, owner.Group)
	}
}

func TestReadFile_FallsBackOnPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("content"), 0o000); err != nil {
		t.Fatal(err)
	}

	tgt, readLog := newCaptureTarget(t)
	_, err := tgt.ReadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("expected fallback to handle it, got: %v", err)
	}

	cmd := readLog()
	if !strings.HasPrefix(cmd, "cat ") {
		t.Fatalf("expected escalated cat, got: %q", cmd)
	}
}

func TestWriteFile_FallsBackOnPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, nil, 0o000); err != nil {
		t.Fatal(err)
	}

	tgt, readLog := newCaptureTarget(t)
	err := tgt.WriteFile(context.Background(), path, []byte("new"))
	if err != nil {
		t.Fatalf("expected fallback to handle it, got: %v", err)
	}

	cmd := readLog()
	if !strings.HasPrefix(cmd, "cp ") {
		t.Fatalf("expected escalated cp, got: %q", cmd)
	}
	if !strings.Contains(cmd, path) {
		t.Fatalf("expected command to reference %s, got: %q", path, cmd)
	}
}

func TestReadFile_NoEscalationErrorWhenNoTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("content"), 0o000); err != nil {
		t.Fatal(err)
	}

	var tgt POSIXTarget // no escalation, not root
	_, err := tgt.ReadFile(context.Background(), path)

	var noEsc target.NoEscalationError
	if !errors.As(err, &noEsc) {
		t.Fatalf("expected NoEscalationError, got %T: %v", err, err)
	}
	if noEsc.Op != "read" {
		t.Fatalf("expected op %q, got %q", "read", noEsc.Op)
	}
	if noEsc.Path != path {
		t.Fatalf("expected path %q, got %q", path, noEsc.Path)
	}
}

func TestWriteFile_NoEscalationErrorWhenNoTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, nil, 0o000); err != nil {
		t.Fatal(err)
	}

	var tgt POSIXTarget // no escalation, not root
	err := tgt.WriteFile(context.Background(), path, []byte("new"))

	var noEsc target.NoEscalationError
	if !errors.As(err, &noEsc) {
		t.Fatalf("expected NoEscalationError, got %T: %v", err, err)
	}
	if noEsc.Op != "write" {
		t.Fatalf("expected op %q, got %q", "write", noEsc.Op)
	}
}
