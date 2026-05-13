// SPDX-License-Identifier: GPL-3.0-only

package fileop

import (
	"testing"

	"scampi.dev/scampi/capability"
)

// TestEnsureModeOp_RequiresFilesystem pins the capability contract for
// EnsureModeOp. It looks impl-detail-shaped (asserts on the cap bitmap)
// but the function it guards is real: if someone refactors the op and
// forgets to declare a required capability, the engine silently routes
// the op against a target that can't service it. This test catches
// that drift at compile-time. Kept deliberately (see #377 discussion).
func TestEnsureModeOp_RequiresFilesystem(t *testing.T) {
	caps := EnsureModeOp{}.RequiredCapabilities()
	if caps&capability.Filesystem == 0 {
		t.Fatal("EnsureModeOp must require Filesystem (uses Stat in Check and Execute)")
	}
	if caps&capability.FileMode == 0 {
		t.Fatal("EnsureModeOp must require FileMode (uses Chmod in Execute)")
	}
}
