// SPDX-License-Identifier: GPL-3.0-only

// gen-stubs generates scampi-lang stub files from the engine registry.
// Uses the same step/target configs the engine uses — always in sync.
package main

import (
	"os"
	"path/filepath"

	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/gen/langstubs"
)

func main() {
	reg := engine.NewRegistry()

	var inputs []langstubs.StubInput
	for _, st := range reg.StepTypes() {
		cfg := st.NewConfig()
		in := langstubs.StubInput{
			Kind:       st.Kind(),
			Config:     cfg,
			OutputType: "Step",
		}
		if ep, ok := cfg.(enumProvider); ok {
			in.Enums = ep.FieldEnumValues()
		}
		inputs = append(inputs, in)
	}
	for _, tt := range reg.TargetTypes() {
		cfg := tt.NewConfig()
		inputs = append(inputs, langstubs.StubInput{
			Kind:       tt.Kind(),
			Config:     cfg,
			OutputType: "Target",
		})
	}

	outDir := "."
	if env := os.Getenv("STUB_OUT_DIR"); env != "" {
		outDir = env
	}

	f, err := os.Create(filepath.Join(outDir, "std.scampi"))
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	if err := langstubs.Generate("std", inputs, langstubs.Options{}, f); err != nil {
		panic(err)
	}
}

type enumProvider interface {
	FieldEnumValues() map[string][]string
}
