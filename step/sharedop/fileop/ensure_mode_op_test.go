// SPDX-License-Identifier: GPL-3.0-only

package fileop

import (
	"testing"

	"scampi.dev/scampi/capability"
)

func TestEnsureModeOp_RequiresFilesystem(t *testing.T) {
	caps := EnsureModeOp{}.RequiredCapabilities()
	if caps&capability.Filesystem == 0 {
		t.Fatal("EnsureModeOp must require Filesystem (uses Stat in Check and Execute)")
	}
	if caps&capability.FileMode == 0 {
		t.Fatal("EnsureModeOp must require FileMode (uses Chmod in Execute)")
	}
}
