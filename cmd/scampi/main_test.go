// SPDX-License-Identifier: GPL-3.0-only

package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "scampi-cli-test-")
	if err != nil {
		panic(err)
	}

	binary = filepath.Join(dir, "scampi")
	out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dir)
		panic("go build failed: " + string(out))
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// run executes the binary with args and returns combined stderr+stdout and exit code.
func run(args ...string) (output string, exitCode int) {
	cmd := exec.Command(binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(out), exitErr.ExitCode()
		}
		return string(out), -1
	}
	return string(out), 0
}

func TestUsageError_GlobalFlagMissingValue(t *testing.T) {
	out, code := run("--color")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "Incorrect Usage") {
		t.Errorf("expected 'Incorrect Usage' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "--color") {
		t.Errorf("expected '--color' in output, got:\n%s", out)
	}
	// Global flag error should show root help with command list.
	if !strings.Contains(out, "COMMANDS") {
		t.Errorf("expected root help (COMMANDS) in output, got:\n%s", out)
	}
}

func TestUsageError_SubcommandFlagMissingValue(t *testing.T) {
	out, code := run("inspect", "--only")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "Incorrect Usage") {
		t.Errorf("expected 'Incorrect Usage' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "--only") {
		t.Errorf("expected '--only' in output, got:\n%s", out)
	}
	// Subcommand flag error should show subcommand help with OPTIONS.
	if !strings.Contains(out, "OPTIONS") {
		t.Errorf("expected subcommand help (OPTIONS) in output, got:\n%s", out)
	}
	// Should mention the subcommand name.
	if !strings.Contains(out, "scampi inspect") {
		t.Errorf("expected 'scampi inspect' in output, got:\n%s", out)
	}
}

func TestUsageError_InvalidColorValue(t *testing.T) {
	out, code := run("--color", "bogus")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "Incorrect Usage") {
		t.Errorf("expected 'Incorrect Usage' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "bogus") {
		t.Errorf("expected 'bogus' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "auto, always, or never") {
		t.Errorf("expected valid values hint in output, got:\n%s", out)
	}
}

func TestUsageError_ColorEatsSubcommand(t *testing.T) {
	// --color with space-separated value that happens to be a subcommand name.
	// The validator should reject "apply" as a color value.
	out, code := run("--color", "apply")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "Incorrect Usage") {
		t.Errorf("expected 'Incorrect Usage' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "apply") {
		t.Errorf("expected 'apply' in output, got:\n%s", out)
	}
}

func TestUsageError_ResolveFlags(t *testing.T) {
	// Verify all resolve flags show help on missing value.
	for _, tt := range []struct {
		cmd  string
		flag string
	}{
		{"apply", "--only"},
		{"apply", "--targets"},
		{"apply", "--env"},
		{"check", "--only"},
		{"plan", "--targets"},
	} {
		t.Run(tt.cmd+"/"+tt.flag, func(t *testing.T) {
			out, code := run(tt.cmd, tt.flag)
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !strings.Contains(out, "Incorrect Usage") {
				t.Errorf("expected 'Incorrect Usage' in output, got:\n%s", out)
			}
			// Should show the subcommand's help, not just the error.
			if !strings.Contains(out, "OPTIONS") {
				t.Errorf("expected subcommand help (OPTIONS) in output, got:\n%s", out)
			}
		})
	}
}

func TestUsageError_ValidUsageStillWorks(t *testing.T) {
	// legend has no required args or flags - should succeed.
	out, code := run("legend")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; output:\n%s", code, out)
	}
}

func TestUsageError_ValidColorFlag(t *testing.T) {
	// --color=never with a working subcommand should succeed.
	out, code := run("--color=never", "legend")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; output:\n%s", code, out)
	}
}

// Color resolution
// -----------------------------------------------------------------------------

// runEnv executes the binary with extra environment variables appended.
func runEnv(env []string, args ...string) string {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), env...)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func TestNoColor_ExplicitAlwaysWins(t *testing.T) {
	out := runEnv([]string{"NO_COLOR=1"}, "--color", "always", "legend")
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("--color always must outrank NO_COLOR, got uncolored output:\n%s", out)
	}
}

func TestNoColor_AutoStaysUncolored(t *testing.T) {
	out := runEnv([]string{"NO_COLOR=1"}, "legend")
	if strings.Contains(out, "\x1b[") {
		t.Errorf("NO_COLOR under the auto default must not color, got:\n%s", out)
	}
}

// CLI fuzzing
// -----------------------------------------------------------------------------

func FuzzCLI(f *testing.F) {
	// ---- Seeds: real, high-value starting points ----
	seeds := []string{
		// bare flags missing values
		"--color",
		"--ascii",
		"-v",
		"-vvvv",

		// invalid flag values
		"--color bogus",
		"--color=",
		"--color=bogus",

		// flag eats subcommand
		"--color apply",
		"--color check",
		"--color inspect",

		// subcommand flags missing values
		"inspect --step",
		"apply --only",
		"apply --targets",
		"apply --only=foo --targets=bar",
		"check --only",

		// unknown flags
		"--nope",
		"apply --nope",
		"inspect --nope",

		// subcommand with no args (where args required)
		"apply",
		"check",
		"inspect",
		"plan",

		// subcommand with too many args
		"index one two",

		// working commands (should exit 0)
		"legend",
		"--color=never legend",
		"--ascii legend",
		"-v legend",
		"index",

		// nonsense
		"",
		"   ",
		"\U0001F389",
		"apply --only=foo --targets=bar nonexistent.scampi",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		args := strings.Fields(input)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, binary, args...)
		out, err := cmd.CombinedOutput()

		if err != nil {
			if ctx.Err() != nil {
				t.Fatalf("TIMEOUT on args %q", input)
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Exit 1 (user error) is expected for bad input.
				// Exit 2 (internal bug / panic) must never happen.
				if exitErr.ExitCode() == 2 {
					t.Fatalf("PANIC (exit 2) on args %q:\n%s", input, out)
				}
				return
			}
		}
	})
}
