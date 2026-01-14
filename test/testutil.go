package test

import (
	"testing"

	"godoit.dev/doit/target"
)

func AssertTargetUntouched(t *testing.T, r *target.Recorder) {
	t.Helper()

	if len(r.Reads) != 0 {
		t.Fatalf(
			"target was read unexpectedly:\n  Reads: %v",
			r.Reads,
		)
	}

	if len(r.Writes) != 0 {
		t.Fatalf(
			"target was written unexpectedly:\n  Writes: %v",
			r.Writes,
		)
	}

	if len(r.Stats) != 0 {
		t.Fatalf(
			"target was stat'ed unexpectedly:\n  Stats: %v",
			r.Stats,
		)
	}

	if len(r.Chmods) != 0 {
		t.Fatalf(
			"target was chmod'ed unexpectedly:\n  Chmods: %v",
			r.Chmods,
		)
	}

	if len(r.Chowns) != 0 {
		t.Fatalf(
			"target was chown'ed unexpectedly:\n  Chowns: %v",
			r.Chowns,
		)
	}
}
