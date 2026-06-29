// SPDX-License-Identifier: GPL-3.0-only

package posix_test

import (
	"context"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/posix"
)

// Regression tests for #314. DetectEscalation must probe sudo/doas with
// `-n` to confirm non-interactive escalation works; otherwise the first
// escalated op hangs silently on a host that requires a password.
func TestDetectEscalation_Cases(t *testing.T) {
	type call struct {
		cmd string
		out target.CommandResult
	}

	type result struct {
		tool   string
		reason target.EscalateReason
	}

	cases := []struct {
		name    string
		isRoot  bool
		runners []call
		want    result
	}{
		{
			name:   "root needs no tool",
			isRoot: true,
			want:   result{tool: "", reason: target.EscalateRoot},
		},
		{
			name: "sudo present and works non-interactively",
			runners: []call{
				{cmd: "command -v sudo", out: target.CommandResult{ExitCode: 0}},
				{cmd: "sudo -n true", out: target.CommandResult{ExitCode: 0}},
			},
			want: result{tool: "sudo", reason: target.EscalateOK},
		},
		{
			name: "sudo present but requires password",
			runners: []call{
				{cmd: "command -v sudo", out: target.CommandResult{ExitCode: 0}},
				{cmd: "sudo -n true", out: target.CommandResult{ExitCode: 1}},
			},
			want: result{tool: "", reason: target.EscalateRequiresPassword},
		},
		{
			name: "no sudo, doas works",
			runners: []call{
				{cmd: "command -v sudo", out: target.CommandResult{ExitCode: 1}},
				{cmd: "command -v doas", out: target.CommandResult{ExitCode: 0}},
				{cmd: "doas -n true", out: target.CommandResult{ExitCode: 0}},
			},
			want: result{tool: "doas", reason: target.EscalateOK},
		},
		{
			name: "neither tool present",
			runners: []call{
				{cmd: "command -v sudo", out: target.CommandResult{ExitCode: 1}},
				{cmd: "command -v doas", out: target.CommandResult{ExitCode: 1}},
			},
			want: result{tool: "", reason: target.EscalateMissing},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := 0
			run := func(_ context.Context, cmd string) (target.CommandResult, error) {
				if idx >= len(tc.runners) {
					t.Fatalf("unexpected command %q", cmd)
				}
				if !strings.Contains(cmd, tc.runners[idx].cmd) {
					t.Fatalf("call %d: cmd %q does not contain %q", idx, cmd, tc.runners[idx].cmd)
				}
				out := tc.runners[idx].out
				idx++
				return out, nil
			}
			tool, reason := posix.DetectEscalation(t.Context(), run, tc.isRoot)
			if tool != tc.want.tool {
				t.Errorf("tool = %q, want %q", tool, tc.want.tool)
			}
			if reason != tc.want.reason {
				t.Errorf("reason = %v, want %v", reason, tc.want.reason)
			}
		})
	}
}
