// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/render/ansi"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/spec"
)

// streamEvents is a deterministic check/apply stream covering the block-render
// cases: multi-field drift (column alignment + op attribution at -vv), a
// signal-only change (header, no rows), a failure, and an unchanged step
// (hidden below -v). Events arrive already ordered, as the Sequencer delivers
// them.
func streamEvents() []event.Event {
	ref := func(i int, kind, desc string) event.StepRef {
		return event.StepRef{Index: i, Kind: kind, Desc: desc}
	}
	chg := func(r event.StepRef, op, field, cur, des string) event.Change {
		return event.Change{Step: r, DisplayID: op, Drift: spec.DriftDetail{Field: field, Current: cur, Desired: des}}
	}
	exec := func(r event.StepRef, op string) event.Change {
		return event.Change{Step: r, Phase: event.ChangeExecuted, DisplayID: op}
	}
	res := func(r event.StepRef, o event.StepOutcome) event.Result {
		return event.Result{Step: r, Outcome: o}
	}

	a0 := ref(0, "dir", "sandbox root")
	a1 := ref(1, "copy", "static page")
	a2 := ref(2, "symlink", "current -> index")
	a3 := ref(3, "run", "drop a marker")
	a4 := ref(4, "service", "restart web")
	a5 := ref(5, "copy", "config file")

	return []event.Event{
		chg(a0, "ensure_mode", "perm", "", "-rwxr-xr-x"),
		chg(a0, "dir", "state", "", "directory"),
		res(a0, event.StepChanged),
		chg(a1, "ensure_mode", "perm", "", "-rw-r--r--"),
		chg(a1, "ensure_owner", "owner:group", "", "pskry:staff"),
		res(a1, event.StepChanged),
		res(a2, event.StepChanged), // signal-only: no drift rows
		chg(a3, "run", "check", "exit 1", "exit 0"),
		res(a3, event.StepFailed),
		// hidden at default, verdict at -v, op-level "satisfied" rows at -vv.
		event.Result{
			Step:    a4,
			Outcome: event.StepUnchanged,
			Ops:     []string{"ensure_service_active", "ensure_service_enabled"},
		},
		// apply-style changed step: executed changes are field-less, so at -vv each
		// op shows its exec/ok glyph (copy_file + ensure_owner ran; ensure_mode was
		// already satisfied). Below -vv it's just the header.
		exec(a5, "ensure_owner"),
		exec(a5, "copy_file"),
		event.Result{
			Step:    a5,
			Outcome: event.StepChanged,
			Ops:     []string{"copy_file", "ensure_mode", "ensure_owner"},
		},
	}
}

// TestStreamGolden locks the check/apply block format (glyph-led header, railed
// drift, aligned columns, (absent), hide-ok, -vv op attribution) across
// verbosities and both glyph sets, plus one ANSI-colored variant so the
// semantic palette (yellow changed / green ok / red failed / blue tags) is a
// pinned contract, not just prose. Regenerate with SCAMPI_UPDATE=1.
func TestStreamGolden(t *testing.T) {
	combos := []struct {
		name string
		v    signal.Verbosity
	}{
		{"quiet", signal.Quiet},
		{"v", signal.V},
		{"vv", signal.VV},
	}
	sets := []struct {
		name  string
		ascii bool
		color signal.ColorMode
	}{
		{"fancy", false, signal.ColorNever},
		{"ascii", true, signal.ColorNever},
		{"color", false, signal.ColorAlways},
	}

	for _, gs := range sets {
		t.Run(gs.name, func(t *testing.T) {
			var b strings.Builder
			for _, combo := range combos {
				var buf bytes.Buffer
				cli := New(Options{
					ColorMode:  gs.color,
					Verbosity:  combo.v,
					ForceASCII: gs.ascii,
					Stdout:     &buf,
					Stderr:     &buf,
				}, nil)
				b.WriteString("=== " + combo.name + " ===\n")
				for _, e := range streamEvents() {
					cli.RenderEvent(e)
				}
				b.WriteString(buf.String())
			}
			compareGolden(t, filepath.Join("testdata", "stream_"+gs.name+".golden"), b.String())
		})
	}
}

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
