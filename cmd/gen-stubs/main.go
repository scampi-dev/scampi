// SPDX-License-Identifier: GPL-3.0-only

// gen-stubs generates scampi stub files from the engine registry.
// Uses the same step/target configs the engine uses — always in sync.
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/gen/langstubs"
)

func main() {
	reg := engine.NewRegistry()

	// Collect all inputs with their full dotted kind.
	var all []langstubs.StubInput
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
		all = append(all, in)
	}
	for _, tt := range reg.TargetTypes() {
		cfg := tt.NewConfig()
		all = append(all, langstubs.StubInput{
			Kind:       tt.Kind(),
			Config:     cfg,
			OutputType: "Target",
		})
	}

	// Group by module: dotted kinds like "rest.request" go to
	// submodule "rest", undotted stay in root "std".
	modules := map[string][]langstubs.StubInput{}
	for _, in := range all {
		mod := ""
		if i := strings.IndexByte(in.Kind, '.'); i >= 0 {
			mod = in.Kind[:i]
			in.Kind = in.Kind[i+1:]
		}
		modules[mod] = append(modules[mod], in)
	}

	outDir := "."
	if env := os.Getenv("STUB_OUT_DIR"); env != "" {
		outDir = env
	}

	for mod, inputs := range modules {
		moduleName := "std"
		filePath := filepath.Join(outDir, "std.scampi")
		if mod != "" {
			moduleName = mod
			dir := filepath.Join(outDir, mod)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				panic(err)
			}
			filePath = filepath.Join(dir, mod+".scampi")
		}

		seen := map[string]bool{}
		var opaqueTypes []string
		for _, in := range inputs {
			if in.OutputType != "" && !seen[in.OutputType] {
				seen[in.OutputType] = true
				opaqueTypes = append(opaqueTypes, in.OutputType)
			}
		}
		sort.Strings(opaqueTypes)

		f, err := os.Create(filePath)
		if err != nil {
			panic(err)
		}
		opts := langstubs.Options{OpaqueTypes: opaqueTypes}
		err = langstubs.Generate(moduleName, inputs, opts, f)
		_ = f.Close()
		if err != nil {
			panic(err)
		}
	}
}

type enumProvider interface {
	FieldEnumValues() map[string][]string
}
