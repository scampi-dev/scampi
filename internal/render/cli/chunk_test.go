// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"bytes"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/signal"
)

// Multi-deploy output inserts a blank line whenever the lane changes, so
// interleaved lanes chunk visually (matters most without color). Consecutive
// same-lane steps stay gap-free.
func TestStreamBlock_BlankLineBetweenLanes(t *testing.T) {
	var buf bytes.Buffer
	c := New(Options{ColorMode: signal.ColorNever, Verbosity: signal.V, Stdout: &buf, Stderr: &buf}, nil)

	step := func(name string, ord, idx int) event.Result {
		return event.Result{
			Step: event.StepRef{
				Deploy: event.DeployRef{Name: name, Ordinal: ord, MaxNameWidth: 4},
				Index:  idx,
				Kind:   "dir",
			},
			Outcome: event.StepChanged,
		}
	}
	// base, base (same lane), dns (switch), base (switch back).
	c.RenderEvent(step("base", 0, 0))
	c.RenderEvent(step("base", 0, 1))
	c.RenderEvent(step("dns", 1, 0))
	c.RenderEvent(step("base", 0, 2))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	blanks := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			blanks++
		}
	}
	if blanks != 2 {
		t.Fatalf("want 2 blank separators (2 lane switches), got %d:\n%q", blanks, lines)
	}
	if strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
		t.Errorf("consecutive same-lane steps must not be separated:\n%q", lines)
	}
}

// Single-deploy runs leave Name empty and never insert lane gaps.
func TestStreamBlock_NoBlankLineSingleDeploy(t *testing.T) {
	var buf bytes.Buffer
	c := New(Options{ColorMode: signal.ColorNever, Verbosity: signal.V, Stdout: &buf, Stderr: &buf}, nil)

	for i := range 3 {
		c.RenderEvent(event.Result{
			Step:    event.StepRef{Index: i, Kind: "dir"},
			Outcome: event.StepChanged,
		})
	}
	if strings.Contains(buf.String(), "\n\n") {
		t.Errorf("single-deploy output should have no lane gaps:\n%q", buf.String())
	}
}
