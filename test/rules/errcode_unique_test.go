// SPDX-License-Identifier: GPL-3.0-only

package rules

import (
	"go/constant"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

// codePackages lists every package that declares errs.Code constants.
// Add new packages here when introducing a new error code namespace.
var codePackages = []string{
	"scampi.dev/scampi/lang/lex",
	"scampi.dev/scampi/lang/parse",
	"scampi.dev/scampi/lang/check",
	"scampi.dev/scampi/linker",
}

// TestDiagnosticCodeUniqueness uses the Go type checker to collect
// every exported constant of type errs.Code across the codebase and
// verifies that no two share the same string value.
func TestDiagnosticCodeUniqueness(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedName,
		Dir:  "../..",
	}
	toLoad := append([]string{"scampi.dev/scampi/errs"}, codePackages...)
	pkgs, err := packages.Load(cfg, toLoad...)
	if err != nil {
		t.Fatal(err)
	}

	// Find the errs.Code type.
	var codeType types.Type
	for _, pkg := range pkgs {
		if pkg.PkgPath == "scampi.dev/scampi/errs" {
			obj := pkg.Types.Scope().Lookup("Code")
			if obj == nil {
				t.Fatal("errs.Code not found")
			}
			codeType = obj.Type()
			break
		}
	}
	if codeType == nil {
		t.Fatal("errs package not loaded")
	}

	seen := map[string]string{} // value → qualified name

	for _, pkg := range pkgs {
		if pkg.PkgPath == "scampi.dev/scampi/errs" {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			c, ok := obj.(*types.Const)
			if !ok {
				continue
			}
			if !types.Identical(c.Type(), codeType) {
				continue
			}
			val := constant.StringVal(c.Val())
			qualName := pkg.Name + "." + name
			if prev, dup := seen[val]; dup {
				t.Errorf("duplicate diagnostic code %q: %s and %s", val, prev, qualName)
			}
			seen[val] = qualName
		}
	}

	if len(seen) == 0 {
		t.Fatal("found no errs.Code constants — check codePackages list")
	}
	t.Logf("checked %d unique diagnostic codes across %d packages", len(seen), len(codePackages))
}
