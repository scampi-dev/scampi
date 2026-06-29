// SPDX-License-Identifier: GPL-3.0-only

package rules

import (
	"go/ast"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

func Test_StepKind_NewConfig_ReturnsPointer(t *testing.T) {
	findStepKind := func(pkgs []*packages.Package) *types.Interface {
		for _, pkg := range pkgs {
			if obj := pkg.Types.Scope().Lookup("StepKind"); obj != nil {
				if tn, ok := obj.(*types.TypeName); ok {
					if iface, ok := tn.Type().Underlying().(*types.Interface); ok {
						return iface
					}
				}
			}
		}
		return nil
	}

	hasNewConfigMethod := func(stepType *types.Interface) bool {
		for method := range stepType.Methods() {
			if method.Name() == "NewConfig" {
				return true
			}
		}
		return false
	}

	cfg := &packages.Config{
		Mode: packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedFiles,
	}

	pkgs, err := packages.Load(cfg, "scampi.dev/...")
	if err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}

	stepType := findStepKind(pkgs)
	if stepType == nil {
		t.Fatal("StepKind interface not found — invariant test is meaningless")
	}

	if !hasNewConfigMethod(stepType) {
		t.Fatal("StepKind no longer defines NewConfig — update invariant test")
	}

	checked := 0
	// Find implementations
	for _, pkg := range pkgs {
		for _, obj := range pkg.TypesInfo.Defs {
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}

			named, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}

			// Skip interfaces (including StepKind itself)
			if _, ok := named.Underlying().(*types.Interface); ok {
				continue
			}

			implements := types.Implements(named, stepType) ||
				types.Implements(types.NewPointer(named), stepType)

			if !implements {
				continue
			}

			// 4. Locate method via method set
			ms := types.NewMethodSet(types.NewPointer(named))
			sel := ms.Lookup(pkg.Types, "NewConfig")
			if sel == nil {
				t.Fatalf("%s implements StepKind but has no NewConfig method", named)
			}

			fnObj := sel.Obj().(*types.Func)

			// 5. Find AST declaration
			for i, file := range pkg.Syntax {
				filename := pkg.GoFiles[i]

				ast.Inspect(file, func(n ast.Node) bool {
					fn, ok := n.(*ast.FuncDecl)
					if !ok || fn.Body == nil {
						return true
					}

					if pkg.TypesInfo.Defs[fn.Name] != fnObj {
						return true
					}

					ast.Inspect(fn.Body, func(n ast.Node) bool {
						ret, ok := n.(*ast.ReturnStmt)
						if !ok {
							return true
						}

						for _, expr := range ret.Results {
							typ := pkg.TypesInfo.TypeOf(expr)
							if typ == nil {
								continue
							}

							if _, ok := typ.(*types.Pointer); !ok {
								p := pkg.Fset.Position(expr.Pos())
								p.Filename = filename

								t.Errorf(
									"%s:%d: %s.NewConfig must return a pointer, got %s",
									p.Filename,
									p.Line,
									named.Obj().Name(),
									typ.String(),
								)
							}
						}
						return true
					})

					return false
				})
			}

			checked++
		}
	}

	if checked == 0 {
		t.Fatal("no StepKind implementations found — invariant test not exercised")
	}
}
