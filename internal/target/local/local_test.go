// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"runtime"
	"testing"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/spec"
)

func TestCreate_DetectsPkgBackend(t *testing.T) {
	tgt, err := Local{}.Create(t.Context(), nil, spec.TargetInstance{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !tgt.Capabilities().HasAll(capability.Pkg) {
		t.Fatalf(
			"expected local target on %s/%s to provide Pkg capability, but it didn't",
			runtime.GOOS,
			runtime.GOARCH,
		)
	}
}
