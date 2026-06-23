// SPDX-License-Identifier: GPL-3.0-only

package osutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"scampi.dev/scampi/internal/errs"
)

// RunDiffTool writes current and desired content to temp files and execs the
// given diff tool interactively. The tool string is split by whitespace to
// support multi-word commands like "nvim -d". The temp dir is cleaned up when
// the tool exits.
//
// diff(1) exit code 1 (files differ) is not treated as an error.
func RunDiffTool(ctx context.Context, tool, destPath string, current, desired []byte) error {
	dir, err := os.MkdirTemp("", "scampi-inspect-")
	if err != nil {
		// bare-error: diff tool infrastructure, only used for CLI display
		return errs.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	base := filepath.Base(destPath)
	currentDir := filepath.Join(dir, "current")
	desiredDir := filepath.Join(dir, "desired")

	if err := os.MkdirAll(currentDir, 0o700); err != nil {
		// bare-error: diff tool infrastructure, only used for CLI display
		return errs.Errorf("creating current dir: %w", err)
	}
	if err := os.MkdirAll(desiredDir, 0o700); err != nil {
		// bare-error: diff tool infrastructure, only used for CLI display
		return errs.Errorf("creating desired dir: %w", err)
	}

	currentFile := filepath.Join(currentDir, base)
	desiredFile := filepath.Join(desiredDir, base)

	if err := os.WriteFile(currentFile, current, 0o600); err != nil {
		// bare-error: diff tool infrastructure, only used for CLI display
		return errs.Errorf("writing current file: %w", err)
	}
	if err := os.WriteFile(desiredFile, desired, 0o600); err != nil {
		// bare-error: diff tool infrastructure, only used for CLI display
		return errs.Errorf("writing desired file: %w", err)
	}

	parts := strings.Fields(tool)
	args := append(parts[1:], currentFile, desiredFile)

	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// diff(1) exits 1 when files differ — not an error for us.
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		// bare-error: diff tool infrastructure, only used for CLI display
		return errs.Errorf("diff tool %q: %w", parts[0], err)
	}

	return nil
}

// ResolveDiffTool picks a diff tool from environment variables.
// Lookup order: SCAMPI_DIFFTOOL → DIFFTOOL → EDITOR → "diff".
func ResolveDiffTool() string {
	for _, env := range []string{"SCAMPI_DIFFTOOL", "DIFFTOOL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "diff"
}
