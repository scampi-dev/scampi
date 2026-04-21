// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/gen"
	"scampi.dev/scampi/test/harness"
)

func TestGenAPI(t *testing.T) {
	root := harness.AbsPath("../testdata/gen-api")
	entries := harness.ReadDirOrDie(root)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dir := filepath.Join(root, name)

		t.Run(name, func(t *testing.T) {
			specPath := findGenSpec(t, dir)
			expectStarPath := filepath.Join(dir, "expected.scampi")
			expectJSONPath := filepath.Join(dir, "expected.json")

			expect := harness.LoadExpected(t, expectJSONPath)
			opts := loadGenOpts(t, expectJSONPath)

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

			var buf bytes.Buffer
			err := gen.API(specPath, "test", &buf, em, opts)

			if expect.Abort {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					t.Fatalf("expected AbortError, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			harness.AssertDiagnostics(t, rec, expect.Diagnostics, specPath)

			if !expect.Abort {
				expectedStar := harness.ReadOrDie(expectStarPath)
				if got := buf.String(); got != string(expectedStar) {
					t.Fatalf(
						"output mismatch:\n--- got ---\n%s\n--- want ---\n%s",
						got,
						expectedStar,
					)
				}
			}
		})
	}
}

func findGenSpec(t *testing.T, dir string) string {
	t.Helper()
	for _, name := range []string{"spec.yaml", "spec.json"} {
		p := filepath.Join(dir, name)
		if _, err := harness.ReadFileSafe(p); err == nil {
			return p
		}
	}
	t.Fatalf("no spec.yaml or spec.json in %s", dir)
	return ""
}

// loadGenOpts reads gen-api options from the expected.json file. The
// "options" key is optional; tests without it get default APIOptions.
func loadGenOpts(t *testing.T, path string) gen.APIOptions {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return gen.APIOptions{}
	}
	var raw struct {
		Options struct {
			NamePrefix string `json:"name_prefix"`
		} `json:"options"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return gen.APIOptions{}
	}
	return gen.APIOptions{
		NamePrefix: raw.Options.NamePrefix,
	}
}
