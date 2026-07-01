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
	// [N] index lines up across lanes.
	const nameW = 7
	ref := func(ord int, lane string, idx int, kind, desc string) event.StepRef {
		return event.StepRef{
			Deploy: event.DeployRef{Name: lane, Ordinal: ord, MaxNameWidth: nameW},
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
