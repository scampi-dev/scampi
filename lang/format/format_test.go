// SPDX-License-Identifier: GPL-3.0-only

package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fixtures live as pairs in testdata/:
//   <name>.scampi.unformatted  — input to scampi fmt (ext keeps `scampi fmt ./...` from rewriting it)
//   <name>.expected.scampi     — golden output

func TestGoldenFiles(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".scampi.unformatted") {
			continue
		}
		base := strings.TrimSuffix(name, ".scampi.unformatted")
		t.Run(base, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata", base+".scampi.unformatted"))
			if err != nil {
				t.Fatal(err)
			}
			golden, err := os.ReadFile(filepath.Join("testdata", base+".expected.scampi"))
			if err != nil {
				t.Fatal(err)
			}

			got, err := Format(input)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}

			if string(got) != string(golden) {
				t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
			}
		})
	}
}

func TestIdempotent(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".expected.scampi") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".expected.scampi")
		t.Run(base, func(t *testing.T) {
			golden, err := os.ReadFile(filepath.Join("testdata", e.Name()))
			if err != nil {
				t.Fatal(err)
			}

			got, err := Format(golden)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}

			if string(got) != string(golden) {
				t.Errorf("formatting golden file is not idempotent:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
			}
		})
	}
}
