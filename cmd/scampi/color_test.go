// SPDX-License-Identifier: GPL-3.0-only

package main_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

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
