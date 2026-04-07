// SPDX-License-Identifier: GPL-3.0-only

package std_test

import (
	"testing"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/std"
)

func TestStdLibCompiles(t *testing.T) {
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	want := []string{"std", "posix", "rest", "container"}
	for _, name := range want {
		if _, ok := modules[name]; !ok {
			t.Errorf("missing module %q", name)
		}
	}
}
