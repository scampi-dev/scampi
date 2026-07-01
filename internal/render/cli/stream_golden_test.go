// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/diagnostic/event"
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
// verbosities and both glyph sets. Regenerate with SCAMPI_UPDATE=1.
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
	}{
		{"fancy", false},
		{"ascii", true},
	}

	for _, gs := range sets {
		t.Run(gs.name, func(t *testing.T) {
			var b strings.Builder
			for _, combo := range combos {
				var buf bytes.Buffer
				cli := New(Options{
					ColorMode:  signal.ColorNever,
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
