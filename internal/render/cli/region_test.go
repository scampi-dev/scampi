// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRegionLines_CapAndOffTTY(t *testing.T) {
	c := New(Options{Stdout: &bytes.Buffer{}, ForceASCII: true}, nil)

	f := newInflight()
	now := time.Unix(0, 0)
	for i := range 4 {
		f.begin(sref(0, "web", i), now)
	}

	// Off-TTY: no live region at all.
	c.isTTY = false
	if got := c.regionLines(f, 0); got != nil {
		t.Errorf("off-TTY region: got %v, want nil", got)
	}

	// TTY: at most 3 running lines + a "(+N more)" overflow line.
	c.isTTY = true
	c.width = 200
	lines := c.regionLines(f, 0)
	if len(lines) != 4 {
		t.Fatalf("region lines: got %d, want 4 (3 capped + overflow)\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[3], "+1 more") {
		t.Errorf("overflow line: %q, want it to mention +1 more", lines[3])
	}
	if !strings.Contains(lines[0], "[1]") || !strings.Contains(lines[0], "web") {
		t.Errorf("first running line: %q, want step [1] tagged web", lines[0])
	}
}

// A finished step drops out of the region; when nothing is running the region is
// empty.
func TestRegionLines_EmptyWhenIdle(t *testing.T) {
	c := New(Options{Stdout: &bytes.Buffer{}, ForceASCII: true}, nil)
	c.isTTY = true
	c.width = 200

	f := newInflight()
	f.begin(sref(0, "web", 0), time.Unix(0, 0))
	f.finish(sref(0, "web", 0))

	if got := c.regionLines(f, 0); len(got) != 0 {
		t.Errorf("idle region: got %v, want empty", got)
	}
}
