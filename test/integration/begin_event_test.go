// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

const beginEventCfg = `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.dir { path = "/var/www" }
  posix.dir { path = "/var/log/app" }
}
`

// TestBeginEvent_FiresPerStepBeforeResult verifies the engine emits one Begin
// per step, and that each step's Begin precedes its Result in the stream. Begin
// is the live region's "step entered execution" signal; Result is its finish.
func TestBeginEvent_FiresPerStepBeforeResult(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, beginEventCfg, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(diagnostic.NewCtx(t.Context(), em)); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	if len(rec.Begins) != 2 {
		t.Fatalf("begins: got %d, want 2 (one per step)\n%s", len(rec.Begins), rec)
	}

	// Every step's Begin must arrive before its Result.
	begun := map[int]bool{}
	for _, ev := range rec.Events {
		switch v := ev.(type) {
		case event.Begin:
			begun[v.Step.Index] = true
		case event.Result:
			if !begun[v.Step.Index] {
				t.Errorf("step %d: Result arrived before Begin", v.Step.Index)
			}
		}
	}
}
