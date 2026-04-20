// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/mod"
	"scampi.dev/scampi/std"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLocalSubmodules_Basic(t *testing.T) {
	dir := t.TempDir()

	// scampi.mod
	writeFile(t, filepath.Join(dir, "scampi.mod"), []byte(
		"module example.com/myproject\n",
	))

	// Root entry point: module main, imports the submodule.
	writeFile(t, filepath.Join(dir, "deploy.scampi"), []byte(`
module main
import "example.com/myproject/targets"
let t = targets.name
`))

	// Subdir module: targets/
	writeFile(t, filepath.Join(dir, "targets", "_index.scampi"), []byte(`
module targets
pub let name = "my-target"
`))

	// Load
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatal(err)
	}

	m, err := mod.Parse(filepath.Join(dir, "scampi.mod"),
		[]byte("module example.com/myproject\n"))
	if err != nil {
		t.Fatal(err)
	}

	userMods := LoadUserModulesFromMod(m, modules)

	// Should have found the targets submodule.
	if _, ok := modules["example.com/myproject/targets"]; !ok {
		t.Fatal("submodule not registered under full path")
	}
	if _, ok := modules["targets"]; !ok {
		t.Fatal("submodule not registered under short name")
	}

	// Should be in the userMods slice for the evaluator.
	found := false
	for _, um := range userMods {
		if um.Name == "targets" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("submodule not in userMods slice")
	}

	// The pub let should be visible, but a hypothetical private
	// one should not be (tested via PublicView filtering).
	scope := modules["example.com/myproject/targets"]
	if scope.Lookup("name") == nil {
		t.Fatal("pub let 'name' should be visible")
	}
}

func TestLoadLocalSubmodules_PrivateNotExported(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "scampi.mod"), []byte(
		"module example.com/proj\n",
	))

	writeFile(t, filepath.Join(dir, "lib", "_index.scampi"), []byte(`
module lib
pub let exported = "yes"
let private = "no"
`))

	modules, _ := check.BootstrapModules(std.FS)
	m, _ := mod.Parse(filepath.Join(dir, "scampi.mod"),
		[]byte("module example.com/proj\n"))

	LoadUserModulesFromMod(m, modules)

	scope := modules["example.com/proj/lib"]
	if scope == nil {
		t.Fatal("submodule not loaded")
	}
	if scope.Lookup("exported") == nil {
		t.Error("pub let should be visible")
	}
	if scope.Lookup("private") != nil {
		t.Error("non-pub let should NOT be visible")
	}
}

func TestLoadLocalSubmodules_SkipsDotDirs(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "scampi.mod"), []byte(
		"module example.com/proj\n",
	))

	// Hidden dir should be skipped.
	writeFile(t, filepath.Join(dir, ".hidden", "_index.scampi"), []byte(`
module hidden
pub let x = 1
`))

	modules, _ := check.BootstrapModules(std.FS)
	m, _ := mod.Parse(filepath.Join(dir, "scampi.mod"),
		[]byte("module example.com/proj\n"))

	LoadUserModulesFromMod(m, modules)

	if _, ok := modules["example.com/proj/.hidden"]; ok {
		t.Error("dot-prefixed dirs should be skipped")
	}
}

func TestLoadLocalSubmodules_SkipsMainModules(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "scampi.mod"), []byte(
		"module example.com/proj\n",
	))

	// Subdir with module main — should not be registered.
	writeFile(t, filepath.Join(dir, "scripts", "run.scampi"), []byte(`
module main
let x = 1
`))

	modules, _ := check.BootstrapModules(std.FS)
	m, _ := mod.Parse(filepath.Join(dir, "scampi.mod"),
		[]byte("module example.com/proj\n"))

	LoadUserModulesFromMod(m, modules)

	if _, ok := modules["example.com/proj/scripts"]; ok {
		t.Error("module main subdirs should not be registered as submodules")
	}
}

func TestLoadLocalSubmodules_Nested(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "scampi.mod"), []byte(
		"module example.com/proj\n",
	))

	writeFile(t, filepath.Join(dir, "lib", "helpers", "_index.scampi"), []byte(`
module helpers
pub func greet() string { return "hi" }
`))

	modules, _ := check.BootstrapModules(std.FS)
	m, _ := mod.Parse(filepath.Join(dir, "scampi.mod"),
		[]byte("module example.com/proj\n"))

	LoadUserModulesFromMod(m, modules)

	if _, ok := modules["example.com/proj/lib/helpers"]; !ok {
		t.Fatal("nested submodule not registered under full path")
	}
}

func TestBrokenSiblingReportedInDiagnostic(t *testing.T) {
	dir := t.TempDir()

	// Good file references a function defined in the broken sibling.
	writeFile(t, filepath.Join(dir, "good.scampi"), []byte(`
module mylib
import "std"
import "std/rest"

pub decl fetch() std.Step {
  return helper()
}
`))

	// Broken sibling — has a lex error (digit-prefixed identifier).
	writeFile(t, filepath.Join(dir, "broken.scampi"), []byte(`
module mylib
import "std"
import "std/rest"

func helper() std.Step {
  let 6bad = 1
  return rest.request("GET", "/api")
}
`))

	modules, _ := check.BootstrapModules(std.FS)
	goodPath := filepath.Join(dir, "good.scampi")

	_, broken := loadSiblingDecls(goodPath, "mylib", modules)
	if len(broken) == 0 {
		t.Fatal("expected broken sibling to be reported")
	}
	if broken[0].path != filepath.Join(dir, "broken.scampi") {
		t.Errorf("broken path = %q, want broken.scampi", broken[0].path)
	}
	if broken[0].firstErr == "" {
		t.Error("broken sibling should carry the first error message")
	}
}
