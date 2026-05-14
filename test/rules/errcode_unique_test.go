// SPDX-License-Identifier: GPL-3.0-only

package rules

import (
	"go/constant"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestDiagnosticCodeUniqueness uses the Go type checker to collect
// every exported constant of type errs.Code across all packages in the
// module and verifies that no two share the same string value.
func TestDiagnosticCodeUniqueness(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedName,
		Dir:  repoRoot(t),
	}
	pkgs, err := packages.Load(cfg, "scampi.dev/scampi/...")
	if err != nil {
		t.Fatal(err)
	}

	// Find the errs.Code type from whichever package exports it.
	var codeType types.Type
	for _, pkg := range pkgs {
		if pkg.PkgPath == "scampi.dev/scampi/errs" {
			obj := pkg.Types.Scope().Lookup("Code")
			if obj != nil {
				codeType = obj.Type()
			}
			break
		}
	}
	if codeType == nil {
		t.Fatal("errs.Code type not found — is scampi.dev/scampi/errs in the module?")
	}

	seen := map[string]string{} // value → qualified name
	scanned := 0

	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.PkgPath, "scampi.dev/scampi/") {
			continue
		}
		if pkg.Types == nil {
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
			scanned++
			val := constant.StringVal(c.Val())
			qualName := pkg.Name + "." + name
			if prev, dup := seen[val]; dup {
				t.Errorf("duplicate diagnostic code %q: %s and %s", val, prev, qualName)
			}
			seen[val] = qualName
		}
	}

	if len(seen) == 0 {
		t.Fatal("found no errs.Code constants — type resolution may have failed")
	}
	t.Logf("checked %d unique diagnostic codes across %d packages", len(seen), scanned)
}
