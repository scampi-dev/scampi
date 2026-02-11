package main_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

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
		"apply --env",
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
		"🎉",
		"apply --only=foo --targets=bar nonexistent.cue",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		args := strings.Fields(input)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
