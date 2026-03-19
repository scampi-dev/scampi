// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/pkgmgr"
)

// newCaptureTarget creates a POSIXTarget whose escalation tool is a
// shell script that records its arguments to a log file instead of
// executing them.  The returned function reads the log.
func newCaptureTarget(t *testing.T) (POSIXTarget, func() string) {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "cmd.log")
	script := filepath.Join(dir, "escalate")

	err := os.WriteFile(script, []byte(
		"#!/bin/sh\necho \"$*\" >> "+logFile+"\n",
	), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	tgt := POSIXTarget{escalate: script}
	readLog := func() string {
		data, _ := os.ReadFile(logFile)
		return strings.TrimSpace(string(data))
	}
	return tgt, readLog
}

// newFailTarget creates a POSIXTarget whose escalation tool always
// fails with exit 1 and writes "denied" to stderr.
func newFailTarget(t *testing.T) POSIXTarget {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fail")

	err := os.WriteFile(script, []byte(
		"#!/bin/sh\necho 'denied' >&2\nexit 1\n",
	), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	return POSIXTarget{escalate: script}
}

// Detection
// -----------------------------------------------------------------------------

func TestDetectEscalation(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root")
	}
	tgt := &POSIXTarget{}
	result := detectEscalation(context.Background(), tgt)
	switch result {
	case "sudo", "doas", "":
	default:
		t.Fatalf("unexpected escalation tool: %q", result)
	}
}

// Stat escalation fallback
// -----------------------------------------------------------------------------

func newStatTarget(t *testing.T, output string) (POSIXTarget, func() string) {
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

	tgt := POSIXTarget{escalate: script}
	readLog := func() string {
		data, _ := os.ReadFile(logFile)
		return strings.TrimSpace(string(data))
	}
	return tgt, readLog
}

func TestStat_FallsBackOnPermission(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root")
	}

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
	if os.Getuid() == 0 {
		t.Skip("requires non-root")
	}

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

// Command construction (capture script records args)
// -----------------------------------------------------------------------------

func TestEscalatedReadFile_Command(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	_, err := tgt.escalatedReadFile(context.Background(), "/etc/shadow")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := readLog(), "cat /etc/shadow"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEscalatedWriteFile_Command(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	err := tgt.escalatedWriteFile(context.Background(), "/etc/config", []byte("data"))
	if err != nil {
		t.Fatal(err)
	}
	got := readLog()
	if !strings.HasPrefix(got, "cp /tmp/.scampi-") {
		t.Fatalf("expected cp from staging path, got %q", got)
	}
	if !strings.HasSuffix(got, " /etc/config") {
		t.Fatalf("expected destination /etc/config, got %q", got)
	}
}

func TestEscalatedRemove_Command(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	err := tgt.escalatedRemove(context.Background(), "/etc/config")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := readLog(), "rm /etc/config"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEscalatedChmod_Command(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	err := tgt.escalatedChmod(context.Background(), "/etc/config", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := readLog(), "chmod 0755 /etc/config"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEscalatedChown_Command(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	err := tgt.escalatedChown(context.Background(), "/etc/config", target.Owner{User: "root", Group: "wheel"})
	if err != nil {
		t.Fatal(err)
	}
	want := "chown root:wheel /etc/config"
	if got := readLog(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEscalatedSymlink_Command(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	err := tgt.escalatedSymlink(context.Background(), "/usr/bin/vim", "/usr/local/bin/vi")
	if err != nil {
		t.Fatal(err)
	}
	want := "ln -sfn /usr/bin/vim /usr/local/bin/vi"
	if got := readLog(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Error types on failure
// -----------------------------------------------------------------------------

func TestEscalatedRemove_ReturnsEscalationError(t *testing.T) {
	tgt := newFailTarget(t)
	err := tgt.escalatedRemove(context.Background(), "/etc/config")

	var escErr target.EscalationError
	if !errors.As(err, &escErr) {
		t.Fatalf("expected EscalationError, got %T: %v", err, err)
	}
	if escErr.Op != "rm" {
		t.Fatalf("expected op %q, got %q", "rm", escErr.Op)
	}
	if escErr.Path != "/etc/config" {
		t.Fatalf("expected path %q, got %q", "/etc/config", escErr.Path)
	}
	if escErr.ExitCode != 1 {
		t.Fatalf("expected exit 1, got %d", escErr.ExitCode)
	}
	if !strings.Contains(escErr.Stderr, "denied") {
		t.Fatalf("expected stderr containing 'denied', got %q", escErr.Stderr)
	}
}

func TestEscalatedReadFile_ReturnsEscalationError(t *testing.T) {
	tgt := newFailTarget(t)
	_, err := tgt.escalatedReadFile(context.Background(), "/etc/shadow")

	var escErr target.EscalationError
	if !errors.As(err, &escErr) {
		t.Fatalf("expected EscalationError, got %T: %v", err, err)
	}
	if escErr.Op != "cat" {
		t.Fatalf("expected op %q, got %q", "cat", escErr.Op)
	}
}

// Try-then-fallback behavior
// -----------------------------------------------------------------------------

func TestReadFile_FallsBackOnPermission(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root")
	}

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
	if os.Getuid() == 0 {
		t.Skip("requires non-root")
	}

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
	if os.Getuid() == 0 {
		t.Skip("requires non-root")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("content"), 0o000); err != nil {
		t.Fatal(err)
	}

	tgt := POSIXTarget{} // no escalation, not root
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
	if os.Getuid() == 0 {
		t.Skip("requires non-root")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, nil, 0o000); err != nil {
		t.Fatal(err)
	}

	tgt := POSIXTarget{} // no escalation, not root
	err := tgt.WriteFile(context.Background(), path, []byte("new"))

	var noEsc target.NoEscalationError
	if !errors.As(err, &noEsc) {
		t.Fatalf("expected NoEscalationError, got %T: %v", err, err)
	}
	if noEsc.Op != "write" {
		t.Fatalf("expected op %q, got %q", "write", noEsc.Op)
	}
}

func TestInstallPkgs_NoEscalationErrorWhenNoTool(t *testing.T) {
	tgt := POSIXTarget{
		pkgBackend: &pkgmgr.Backend{
			Kind:      pkgmgr.Apk,
			Install:   "apk add %s",
			NeedsRoot: true,
		},
		// escalate is "", isRoot is false (zero values)
	}

	err := tgt.InstallPkgs(context.Background(), []string{"curl"})

	var noEsc target.NoEscalationError
	if !errors.As(err, &noEsc) {
		t.Fatalf("expected NoEscalationError, got %T: %v", err, err)
	}
	if noEsc.Op != "apk install" {
		t.Fatalf("expected op %q, got %q", "apk install", noEsc.Op)
	}
}

func TestRemovePkgs_NoEscalationErrorWhenNoTool(t *testing.T) {
	tgt := POSIXTarget{
		pkgBackend: &pkgmgr.Backend{
			Kind:      pkgmgr.Apt,
			Remove:    "apt-get remove -y %s",
			NeedsRoot: true,
		},
	}

	err := tgt.RemovePkgs(context.Background(), []string{"nginx"})

	var noEsc target.NoEscalationError
	if !errors.As(err, &noEsc) {
		t.Fatalf("expected NoEscalationError, got %T: %v", err, err)
	}
	if noEsc.Op != "apt remove" {
		t.Fatalf("expected op %q, got %q", "apt remove", noEsc.Op)
	}
}

func TestInstallPkgs_NoErrorWhenRoot(t *testing.T) {
	tgt, _ := newCaptureTarget(t)
	tgt.isRoot = true
	tgt.pkgBackend = &pkgmgr.Backend{
		Install:   "echo install %s",
		NeedsRoot: true,
	}

	err := tgt.InstallPkgs(context.Background(), []string{"curl"})
	if err != nil {
		t.Fatalf("expected no error when root, got: %v", err)
	}
}

// Package manager escalation
// -----------------------------------------------------------------------------

func TestInstallPkgs_Escalated(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	tgt.pkgBackend = &pkgmgr.Backend{
		Install:   "echo install %s",
		NeedsRoot: true,
	}

	err := tgt.InstallPkgs(context.Background(), []string{"nginx"})
	if err != nil {
		t.Fatal(err)
	}

	cmd := readLog()
	if !strings.Contains(cmd, "echo install") {
		t.Fatalf("expected escalated install, got: %q", cmd)
	}
}

func TestRemovePkgs_Escalated(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	tgt.pkgBackend = &pkgmgr.Backend{
		Remove:    "echo remove %s",
		NeedsRoot: true,
	}

	err := tgt.RemovePkgs(context.Background(), []string{"nginx"})
	if err != nil {
		t.Fatal(err)
	}

	cmd := readLog()
	if !strings.Contains(cmd, "echo remove") {
		t.Fatalf("expected escalated remove, got: %q", cmd)
	}
}

func TestInstallPkgs_NoEscalationWithoutNeedsRoot(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	tgt.pkgBackend = &pkgmgr.Backend{
		Install:   "echo install %s",
		NeedsRoot: false,
	}

	err := tgt.InstallPkgs(context.Background(), []string{"wget"})
	if err != nil {
		t.Fatal(err)
	}

	if log := readLog(); log != "" {
		t.Fatalf("expected no escalation, got: %q", log)
	}
}

// UpdateCache escalation
// -----------------------------------------------------------------------------

func TestUpdateCache_NoEscalationErrorWhenNoTool(t *testing.T) {
	tgt := POSIXTarget{
		pkgBackend: &pkgmgr.Backend{
			Kind:           pkgmgr.Apt,
			UpdateCache:    "apt-get update -qq",
			IsUpgradable:   "apt list --upgradable %s",
			CacheNeedsRoot: true,
		},
	}

	err := tgt.UpdateCache(context.Background())

	var noEsc target.NoEscalationError
	if !errors.As(err, &noEsc) {
		t.Fatalf("expected NoEscalationError, got %T: %v", err, err)
	}
	if noEsc.Op != "apt update-cache" {
		t.Fatalf("expected op %q, got %q", "apt update-cache", noEsc.Op)
	}
}

func TestUpdateCache_Escalated(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	tgt.pkgBackend = &pkgmgr.Backend{
		UpdateCache:    "echo update-cache",
		IsUpgradable:   "true %s",
		CacheNeedsRoot: true,
	}

	err := tgt.UpdateCache(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	cmd := readLog()
	if !strings.Contains(cmd, "echo update-cache") {
		t.Fatalf("expected escalated update-cache, got: %q", cmd)
	}
}

func TestUpdateCache_NoEscalationWithoutCacheNeedsRoot(t *testing.T) {
	tgt, readLog := newCaptureTarget(t)
	tgt.pkgBackend = &pkgmgr.Backend{
		UpdateCache:    "echo update-cache",
		IsUpgradable:   "true %s",
		CacheNeedsRoot: false,
	}

	err := tgt.UpdateCache(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if log := readLog(); log != "" {
		t.Fatalf("expected no escalation, got: %q", log)
	}
}

func TestUpdateCache_NoUpgradeSupport(t *testing.T) {
	tgt := POSIXTarget{
		pkgBackend: &pkgmgr.Backend{
			Kind:        pkgmgr.Apt,
			IsInstalled: "dpkg -s %s",
			Install:     "apt-get install -y %s",
			Remove:      "apt-get remove -y %s",
		},
	}

	err := tgt.UpdateCache(context.Background())
	if err == nil || !strings.Contains(err.Error(), "BUG") {
		t.Fatalf("expected BUG error, got: %v", err)
	}

	_, err = tgt.IsUpgradable(context.Background(), "foo")
	if err == nil || !strings.Contains(err.Error(), "BUG") {
		t.Fatalf("expected BUG error, got: %v", err)
	}
}

func TestUpdateCache_CacheUpdateError(t *testing.T) {
	tgt := newFailTarget(t)
	tgt.pkgBackend = &pkgmgr.Backend{
		UpdateCache:    "false",
		IsUpgradable:   "true %s",
		CacheNeedsRoot: false,
	}

	err := tgt.UpdateCache(context.Background())

	var cacheErr CacheUpdateError
	if !errors.As(err, &cacheErr) {
		t.Fatalf("expected CacheUpdateError, got %T: %v", err, err)
	}
	if cacheErr.ExitCode != 1 {
		t.Fatalf("expected exit 1, got %d", cacheErr.ExitCode)
	}
}
