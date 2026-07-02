// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"testing"

	"scampi.dev/scampi/internal/target"
)

// Registry dedup
// -----------------------------------------------------------------------------

func Test_AddMemTarget_DedupesByName(t *testing.T) {
	reg := NewTestRegistry()
	entry1 := reg.AddMemTarget(MemTargetEntry{
		Name: "m",
		Mock: target.NewMemTarget(),
	})
	entry2 := reg.AddMemTarget(MemTargetEntry{
		Name: "m",
		Mock: target.NewMemTarget(),
	})
	if entry1.Mock != entry2.Mock {
		t.Errorf("second AddMemTarget should return the first mock")
	}
	if len(reg.MemTargets()) != 1 {
		t.Errorf("expected 1 entry, got %d", len(reg.MemTargets()))
	}
}
