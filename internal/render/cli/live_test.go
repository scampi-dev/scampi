// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/render/ansi"
	"scampi.dev/scampi/internal/signal"
)

// newTTYCLI builds a CLI that renders its live region into buf with a frozen
// clock, so the arena is deterministic and assertable off a real terminal. This
// is the seam that makes the live region testable: TTY-ness and the clock are
// forced rather than derived from an *os.File.
func newTTYCLI(at time.Time) (*CLI, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	c := New(Options{Stdout: buf, ColorMode: signal.ColorNever, ForceASCII: true}, nil)
	c.isTTY = true
	c.sink.tty = true
	c.width = 200
	c.now = func() time.Time { return at }
	return c, buf
}

// The sink erases the pinned region, writes durable output, then redraws the
// region beneath it, so scrollback stays clean and the region stays at the
// bottom.
func TestSink_RegionEraseRedraw(t *testing.T) {
	buf := &bytes.Buffer{}
	s := newSink(buf, buf, true)

	s.setRegion([]string{"region line"})
	s.emit([]renderEvent{{stream: streamOut, line: "durable"}})

	out := buf.String()
	if n := strings.Count(out, "region line"); n != 2 {
		t.Errorf("region drawn %d times, want 2 (initial + redraw after durable)", n)
	}
	if !strings.Contains(out, ansi.CursorUp(1)) || !strings.Contains(out, ansi.EraseToEnd) {
		t.Errorf("expected cursor-up + erase around the durable write, got %q", out)
	}
	first := strings.Index(out, "region line")
	durable := strings.Index(out, "durable")
	last := strings.LastIndex(out, "region line")
	if first >= durable || durable >= last {
		t.Errorf("durable line should sit between the erased and redrawn region: %q", out)
	}

	// clearRegion wipes it for good.
	buf.Reset()
	s.clearRegion()
	if got := buf.String(); !strings.Contains(got, ansi.EraseToEnd) {
		t.Errorf("clearRegion should erase the region, got %q", got)
	}
}

// End-to-end: driving a scripted event stream through the stream sink
// synchronously (no goroutine) renders durable tagged blocks with the live
// region drawn, erased, and redrawn around them, and wiped at finish.
func TestStream_EndToEndLiveRegion(t *testing.T) {
	at := time.Unix(1000, 0)
	c, buf := newTTYCLI(at)
	s := newStreamSink(c)

	web := func(idx int) event.StepRef {
		return event.StepRef{Deploy: event.DeployRef{Name: "web", Ordinal: 0}, Index: idx, Kind: "dir"}
	}

	s.handle(event.Begin{Step: web(0)})
	s.handle(event.Begin{Step: web(1)})
	s.handle(event.Result{Step: web(0), Outcome: event.StepChanged})
	s.handle(event.Result{Step: web(1), Outcome: event.StepChanged})
	s.finish()

	out := buf.String()

	// Durable blocks: tagged, 1-based, in lane order.
	if !strings.Contains(out, "[web]") {
		t.Errorf("missing deploy tag in durable output:\n%q", out)
	}
	if !strings.Contains(out, "[1] dir") || !strings.Contains(out, "[2] dir") {
		t.Errorf("missing tagged step blocks:\n%q", out)
	}
	// The live region was active: it drew a running step (elapsed 0s under the
	// frozen clock) and got erased at least once.
	if !strings.Contains(out, "(0s)") {
		t.Errorf("region never drew a running step (no elapsed):\n%q", out)
	}
	if !strings.Contains(out, ansi.EraseToEnd) {
		t.Errorf("region was never erased/redrawn:\n%q", out)
	}
}
