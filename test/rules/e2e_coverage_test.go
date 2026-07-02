// SPDX-License-Identifier: GPL-3.0-only

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/engine"
)

// Test_Rule_EveryStepKindHasE2EFixture requires at least one e2e scenario
// per registered step kind (#444). The e2e suite is the primary behavioral
// contract for steps: config in, converged state out. A step kind without a
// fixture ships on integration tests alone, which mock the wiring instead
// of exercising the scampi -> plan -> check -> apply pipeline.
//
// A fixture "covers" a kind when its config.scampi invokes it: bare kinds
// are matched as ".<kind> " (module-qualified invocation, e.g. posix.copy),
// dotted kinds as the full name (e.g. container.instance).
func Test_Rule_EveryStepKindHasE2EFixture(t *testing.T) {
	root := repoRoot(t)
	e2eRoot := filepath.Join(root, "test", "testdata", "e2e")

	entries, err := os.ReadDir(e2eRoot)
	if err != nil {
		t.Fatalf("read e2e fixture root: %v", err)
	}

	var configs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(e2eRoot, e.Name(), "config.scampi"))
		if err != nil {
			continue // missing config.scampi is the e2e suite's problem
		}
		configs = append(configs, string(data))
	}

	invoked := func(kind string) bool {
		needle := "." + kind + " "
		if strings.Contains(kind, ".") {
			needle = kind + " "
		}
		for _, cfg := range configs {
			if strings.Contains(cfg, needle) {
				return true
			}
		}
		return false
	}

	reg := engine.NewRegistry()
	for _, st := range reg.StepKinds() {
		kind := st.Kind()
		if !invoked(kind) {
			t.Errorf("step kind %q has no e2e fixture - add a scenario under "+
				"test/testdata/e2e/ that invokes it (config.scampi + source.json + expect.json)",
				kind)
		}
	}
}
