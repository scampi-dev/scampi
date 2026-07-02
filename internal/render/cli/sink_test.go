// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"bytes"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/render/ansi"
)

// The sink erases the pinned region, writes durable output, then redraws the
// region beneath it, so scrollback stays clean and the region stays at the
// bottom.
func Test_Sink_RegionEraseRedraw(t *testing.T) {
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
