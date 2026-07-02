// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/signal"
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
	f.finish(sref(0, "web", 0), false)

	if got := c.regionLines(f, 0); len(got) != 0 {
		t.Errorf("idle region: got %v, want empty", got)
	}
}

// A region taller than the terminal breaks CursorUp math (it clamps at the top
// row), so lane lines get cut to fit; the progress footer always survives.
func TestRegionLines_HeightCap(t *testing.T) {
	c := New(Options{Stdout: &bytes.Buffer{}, ForceASCII: true}, nil)
	c.isTTY = true
	c.width = 200
	c.height = 5

	f := newInflight()
	now := time.Unix(0, 0)
	// 4 lanes x 1 running step + footer = 5 lines, over the 4-line budget.
	for ord, name := range []string{"web", "dns", "npm", "db"} {
		ref := sref(ord, name, 0)
		ref.Deploy.RunTotalSteps = 8
		f.begin(ref, now)
	}

	lines := c.regionLines(f, 0)
	if len(lines) > 4 {
		t.Fatalf("region exceeds height budget: %d lines for a 5-row terminal\n%s",
			len(lines), strings.Join(lines, "\n"))
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, "0/8 steps") {
		t.Errorf("footer should survive the cap, last line: %q", last)
	}
}

// TestLiveRegionGolden pins one rendered live-region frame: spinner, padded
// deploy tags (aligned [N] indexes across lanes of differing name width),
// elapsed under a frozen clock, and the per-lane cap collapsing the overflow
// into "(+N more)". Two frames so the spinner advance is visible. The region is
// ephemeral (erased and redrawn every repaint), so this golden may churn more
// than the durable stream_*.golden -- drop it if it stops earning its keep.
// Regenerate with SCAMPI_UPDATE=1.
func TestLiveRegionGolden(t *testing.T) {
	base := time.Unix(1000, 0)
	frozen := base.Add(5 * time.Second)

	// Widest lane name is "gateway" (7); set it on every ref so tags pad and the
	// [N] index lines up across lanes. total is the run-wide step count for the
	// N-of-M progress footer.
	const nameW = 7
	const total = 9
	ref := func(ord int, lane string, idx int, kind, desc string) event.StepRef {
		return event.StepRef{
			Deploy: event.DeployRef{Name: lane, Ordinal: ord, MaxNameWidth: nameW, RunTotalSteps: total},
			Index:  idx,
			Kind:   kind,
			Desc:   desc,
		}
	}

	build := func() *inflight {
		f := newInflight()
		f.begin(ref(0, "web", 0, "dir", "sandbox root"), base)
		f.begin(ref(0, "web", 1, "copy", "static page"), base.Add(time.Second))
		// gateway lane exceeds the 3-per-lane cap -> "(+1 more)".
		f.begin(ref(1, "gateway", 0, "run", "reload"), base.Add(2*time.Second))
		f.begin(ref(1, "gateway", 1, "user", "svc acct"), base.Add(3*time.Second))
		f.begin(ref(1, "gateway", 2, "pkg", "nginx"), base.Add(4*time.Second))
		f.begin(ref(1, "gateway", 3, "service", "restart"), base.Add(5*time.Second))
		// Two already finished -> footer shows "2/9 steps".
		f.begin(ref(0, "web", 4, "symlink", "current"), base)
		f.finish(ref(0, "web", 4, "symlink", "current"), false)
		f.begin(ref(0, "web", 5, "run", "warm cache"), base)
		f.finish(ref(0, "web", 5, "run", "warm cache"), false)
		// A settled hook counts outside the plan total -> "+1 hook" suffix.
		f.begin(ref(0, "web", 6, "service", "reload nginx"), base)
		f.finish(ref(0, "web", 6, "service", "reload nginx"), true)
		return f
	}

	sets := []struct {
		name  string
		ascii bool
	}{
		{"fancy", false},
		{"ascii", true},
	}
	for _, gs := range sets {
		t.Run(gs.name, func(t *testing.T) {
			c := New(Options{ColorMode: signal.ColorNever, ForceASCII: gs.ascii, Stdout: &bytes.Buffer{}}, nil)
			c.isTTY = true
			c.width = 200
			c.now = func() time.Time { return frozen }

			f := build()
			var b strings.Builder
			for _, frame := range []int{0, 1} {
				b.WriteString("=== frame " + strconv.Itoa(frame) + " ===\n")
				for _, line := range c.regionLines(f, frame) {
					b.WriteString(line + "\n")
				}
			}
			compareGolden(t, filepath.Join("testdata", "region_"+gs.name+".golden"), b.String())
		})
	}
}
