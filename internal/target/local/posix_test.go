// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"context"
	"testing"
)

func Test_RunCommand_ReturnsStdoutOnSuccess(t *testing.T) {
	var tgt POSIXTarget
	result, err := tgt.RunCommand(t.Context(), "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Fatalf("expected stdout %q, got %q", "hello\n", result.Stdout)
	}
}

func Test_RunCommand_ReturnsNonZeroExitOnFailure(t *testing.T) {
	var tgt POSIXTarget
	result, err := tgt.RunCommand(t.Context(), "false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func Test_RunCommand_ReturnsStderr(t *testing.T) {
	var tgt POSIXTarget
	result, err := tgt.RunCommand(t.Context(), "echo oops >&2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Stderr != "oops\n" {
		t.Fatalf("expected stderr %q, got %q", "oops\n", result.Stderr)
	}
}

func Test_RunCommand_HonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var tgt POSIXTarget
	_, err := tgt.RunCommand(ctx, "sleep 60")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
