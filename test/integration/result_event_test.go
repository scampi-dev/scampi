// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

const resultEventCfg = `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.dir { path = "/var/www" }
}
`

// TestResultEvent_Apply verifies the engine emits one Result per action as it
// settles, with the verdict reflecting whether the step changed anything.
func TestResultEvent_Apply(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, resultEventCfg, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(diagnostic.NewCtx(context.Background(), em)); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	if len(rec.Results) != 1 {
		t.Fatalf("results: got %d, want 1\n%s", len(rec.Results), rec)
	}
	got := rec.Results[0]
	if got.Outcome != event.StepChanged {
		t.Errorf("outcome: got %v, want StepChanged", got.Outcome)
	}
	if got.Step.Kind != "dir" {
		t.Errorf("step kind: got %q, want %q", got.Step.Kind, "dir")
	}
	if got.Summary.Changed == 0 {
		t.Errorf("summary.Changed: got 0, want > 0")
	}
}

// TestResultEvent_CheckWouldChange verifies that in check mode an unsatisfied
// step reports StepChanged (would change), driven by the WouldChange count.
func TestResultEvent_CheckWouldChange(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, resultEventCfg, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if _, err = e.Check(diagnostic.NewCtx(context.Background(), em)); err != nil {
		t.Fatalf("Check: %v\n%s", err, rec)
	}

	if len(rec.Results) != 1 {
		t.Fatalf("results: got %d, want 1\n%s", len(rec.Results), rec)
	}
	if got := rec.Results[0]; got.Outcome != event.StepChanged {
		t.Errorf("outcome: got %v, want StepChanged (would change)", got.Outcome)
	}
}
