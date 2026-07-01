// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"testing"
	"time"

	"scampi.dev/scampi/internal/diagnostic/event"
)

func sref(ord int, name string, idx int) event.StepRef {
	return event.StepRef{Deploy: event.DeployRef{Name: name, Ordinal: ord}, Index: idx}
}

func TestInflight_BeginFinishPerLane(t *testing.T) {
	f := newInflight()
	base := time.Unix(0, 0)

	f.begin(sref(0, "web", 0), base)
	f.begin(sref(0, "web", 1), base.Add(time.Second))
	f.begin(sref(1, "dns", 0), base)

	if !f.anyRunning() {
		t.Fatal("expected running steps")
	}

	v := f.view()
	if len(v) != 2 {
		t.Fatalf("lanes: got %d, want 2", len(v))
	}
	// First-seen order: web (ord 0) then dns (ord 1).
	if v[0].Name != "web" || v[1].Name != "dns" {
		t.Fatalf("lane order: got %q,%q want web,dns", v[0].Name, v[1].Name)
	}
	if len(v[0].Running) != 2 {
		t.Fatalf("web running: got %d, want 2", len(v[0].Running))
	}
	// Longest-running first: web[0] started before web[1].
	if v[0].Running[0].ref.Index != 0 {
		t.Errorf("web running[0]: got index %d, want 0 (oldest first)", v[0].Running[0].ref.Index)
	}

	// Finish web[0]: leaves running, bumps finished.
	f.finish(sref(0, "web", 0))
	v = f.view()
	if len(v[0].Running) != 1 || v[0].Running[0].ref.Index != 1 {
		t.Errorf("after finish: web running = %+v, want only index 1", v[0].Running)
	}
	if v[0].Finished != 1 {
		t.Errorf("web finished: got %d, want 1", v[0].Finished)
	}
}

func TestInflight_AllFinished(t *testing.T) {
	f := newInflight()
	base := time.Unix(0, 0)
	f.begin(sref(0, "web", 0), base)
	f.finish(sref(0, "web", 0))
	if f.anyRunning() {
		t.Error("expected nothing running after finish")
	}
}

func TestInflight_Progress(t *testing.T) {
	f := newInflight()
	base := time.Unix(0, 0)
	step := func(ord, idx int) event.StepRef {
		return event.StepRef{Deploy: event.DeployRef{Ordinal: ord, RunTotalSteps: 5}, Index: idx}
	}
	f.begin(step(0, 0), base)
	f.begin(step(0, 1), base)
	f.begin(step(1, 0), base)

	if done, total := f.progress(); done != 0 || total != 5 {
		t.Fatalf("before any finish: got %d/%d, want 0/5", done, total)
	}
	// Finish across two lanes: done sums lane-wide.
	f.finish(step(0, 0))
	f.finish(step(1, 0))
	if done, total := f.progress(); done != 2 || total != 5 {
		t.Errorf("after 2 finishes: got %d/%d, want 2/5", done, total)
	}
}
