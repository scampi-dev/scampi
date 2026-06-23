// SPDX-License-Identifier: GPL-3.0-only

package rules

import (
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	template "scampi.dev/scampi/internal/render/template"
)

// TestAllTemplatesRender is a contract test that auto-discovers every
// diagnostic.Raisable and spec.OpDescription implementation in the module,
// extracts their template string literals from the AST, resolves the Data
// type via go/types, and renders each template with both populated and nil
// data. A panic means a template references a field that doesn't exist on
// the Data struct.
//
// Adding a new Raisable or OpDescription type is automatically picked up
// — no manual registration.
func TestAllTemplatesRender(t *testing.T) {
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

	diagnosticIface := findInterface(pkgs, "Raisable")
	if diagnosticIface == nil {
		t.Fatal("diagnostic.Raisable interface not found")
	}

	opDescIface := findInterface(pkgs, "OpDescription")
	if opDescIface == nil {
		t.Fatal("spec.OpDescription interface not found")
	}

	var all []renderable
	checked := 0

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

			if _, ok := named.Underlying().(*types.Interface); ok {
				continue
			}

			isDiag := types.Implements(named, diagnosticIface) ||
				types.Implements(types.NewPointer(named), diagnosticIface)
			isOpDesc := types.Implements(named, opDescIface) ||
				types.Implements(types.NewPointer(named), opDescIface)

			if !isDiag && !isOpDesc {
				continue
			}

			checked++

			if isDiag {
				rr := extractDiagnosticTemplates(t, pkg, named)
				all = append(all, rr...)
			}
			if isOpDesc {
				rr := extractOpDescTemplates(t, pkg, named)
				all = append(all, rr...)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no Diagnostic or OpDescription implementations found")
	}

	t.Logf("discovered %d implementors, %d renderable templates", checked, len(all))

	for _, r := range all {
		t.Run(r.id, func(t *testing.T) {
			defer func() {
				if v := recover(); v != nil {
					t.Fatalf("[%s] template.Render panicked: %v", r.typeName, v)
				}
			}()
			template.Render(testRenderable{id: r.id, text: r.text, data: r.data})
		})
	}
}

type renderable struct {
	typeName string
	id       string
	text     string
	data     any
}

type testRenderable struct {
	id   string
	text string
	data any
}

func (r testRenderable) TemplateID() string   { return r.id }
func (r testRenderable) TemplateText() string { return r.text }
func (r testRenderable) TemplateData() any    { return r.data }

// findInterface finds a named interface type across all loaded packages.
func findInterface(pkgs []*packages.Package, name string) *types.Interface {
	for _, pkg := range pkgs {
		obj := pkg.Types.Scope().Lookup(name)
		if obj == nil {
			continue
		}
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		iface, ok := tn.Type().Underlying().(*types.Interface)
		if !ok {
			continue
		}
		return iface
	}
	return nil
}

// extractDiagnosticTemplates finds the Diagnostic() method of a Raisable
// implementor and extracts template literals from the returned event.Template{}.
func extractDiagnosticTemplates(t *testing.T, pkg *packages.Package, named *types.Named) []renderable {
	t.Helper()
	return extractFromMethod(t, pkg, named, "Diagnostic", []string{"Text", "Hint", "Help"})
}

// extractOpDescTemplates finds the PlanTemplate() method of an OpDescription
// implementor and extracts template literals from the returned spec.PlanTemplate{}.
func extractOpDescTemplates(t *testing.T, pkg *packages.Package, named *types.Named) []renderable {
	t.Helper()
	return extractFromMethod(t, pkg, named, "PlanTemplate", []string{"Text"})
}

// extractFromMethod locates the named method on the given type, finds the
// composite literal in its return statement, extracts string literal fields,
// resolves the Data type, builds test data, and returns renderables.
func extractFromMethod(
	t *testing.T,
	pkg *packages.Package,
	named *types.Named,
	methodName string,
	fieldNames []string,
) []renderable {
	t.Helper()

	typeName := named.Obj().Pkg().Name() + "." + named.Obj().Name()

	// Find the method via method set
	ms := types.NewMethodSet(named)
	sel := ms.Lookup(pkg.Types, methodName)
	if sel == nil {
		ms = types.NewMethodSet(types.NewPointer(named))
		sel = ms.Lookup(pkg.Types, methodName)
	}
	if sel == nil {
		t.Logf("SKIP %s: no %s method found", typeName, methodName)
		return nil
	}

	fnObj := sel.Obj().(*types.Func)

	// Walk AST to find the method declaration
	var results []renderable

	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}

			if pkg.TypesInfo.Defs[fn.Name] != fnObj {
				return true
			}

			// Found the method — look for composite literal in return statements
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ret, ok := n.(*ast.ReturnStmt)
				if !ok {
					return true
				}

				for _, expr := range ret.Results {
					cl := findCompositeLit(expr)
					if cl == nil {
						continue
					}

					// Drill into the Template field of an event.Error /
					// event.Warning / event.Info wrapper so we operate on
					// the Template literal regardless of whether it was
					// returned bare (PlanTemplate) or wrapped (Diagnostic).
					if inner := findTemplateField(cl); inner != nil {
						cl = inner
					}

					fields := extractStringFields(t, typeName, cl, fieldNames)
					dataType := extractDataType(pkg, cl)
					data := buildTestData(dataType)

					// Extract the ID field
					idStr := extractStringField(pkg, cl, "ID")

					for fieldName, text := range fields {
						if text == "" {
							continue
						}
						id := idStr + "." + fieldName
						if idStr == "" {
							id = typeName + "." + fieldName
						}
						results = append(results, renderable{
							typeName: typeName,
							id:       id,
							text:     text,
							data:     data,
						})
					}
				}
				return true
			})

			return false
		})
	}

	return results
}

// findTemplateField returns the composite literal assigned to a
// "Template" field inside cl, or nil if no such field exists. Used to
// drill from a Raisable's outer event.{Error,Warning,Info} wrapper
// into the embedded event.Template literal.
func findTemplateField(cl *ast.CompositeLit) *ast.CompositeLit {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok || ident.Name != "Template" {
			continue
		}
		return findCompositeLit(kv.Value)
	}
	return nil
}

// findCompositeLit unwraps an expression to find the composite literal.
// Handles &T{} and T{} patterns.
func findCompositeLit(expr ast.Expr) *ast.CompositeLit {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return e
	case *ast.UnaryExpr:
		if cl, ok := e.X.(*ast.CompositeLit); ok {
			return cl
		}
	}
	return nil
}

// extractStringFields extracts string literal values from a composite literal
// for the given field names. Fails the test if a wanted field is present but
// is not a string literal — all template text must be statically analyzable.
func extractStringFields(t *testing.T, typeName string, cl *ast.CompositeLit, fieldNames []string) map[string]string {
	t.Helper()

	result := make(map[string]string)
	wanted := make(map[string]bool, len(fieldNames))
	for _, name := range fieldNames {
		wanted[name] = true
	}

	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if !wanted[ident.Name] {
			continue
		}
		s, ok := resolveStringExpr(kv.Value)
		if !ok {
			t.Errorf("%s: field %s must be a string literal, got %T", typeName, ident.Name, kv.Value)
			continue
		}
		result[ident.Name] = s
	}
	return result
}

// extractStringField extracts a single string literal field value.
func extractStringField(_ *packages.Package, cl *ast.CompositeLit, fieldName string) string {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok || ident.Name != fieldName {
			continue
		}
		lit, ok := kv.Value.(*ast.BasicLit)
		if !ok {
			continue
		}
		return stripQuotes(lit.Value)
	}
	return ""
}

// extractDataType resolves the Go type of the Data field expression.
func extractDataType(pkg *packages.Package, cl *ast.CompositeLit) types.Type {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok || ident.Name != "Data" {
			continue
		}
		return pkg.TypesInfo.TypeOf(kv.Value)
	}
	return nil
}

// buildTestData creates a map[string]any with representative test values
// for every exported field of the given struct type.
func buildTestData(typ types.Type) any {
	if typ == nil {
		return nil
	}
	return buildTestDataFromType(typ, 0)
}

func buildTestDataFromType(typ types.Type, depth int) any {
	if depth > 5 {
		return nil
	}

	// Unwrap pointer
	if ptr, ok := typ.(*types.Pointer); ok {
		return buildTestDataFromType(ptr.Elem(), depth)
	}

	// Unwrap named types
	if named, ok := typ.(*types.Named); ok {
		return buildTestDataFromType(named.Underlying(), depth)
	}

	st, ok := typ.(*types.Struct)
	if !ok {
		return testValueForType(typ, depth)
	}

	m := make(map[string]any)
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		m[f.Name()] = testValueForType(f.Type(), depth+1)
	}
	return m
}

func testValueForType(typ types.Type, depth int) any {
	if depth > 5 {
		return nil
	}

	// Unwrap pointer
	if ptr, ok := typ.(*types.Pointer); ok {
		v := testValueForType(ptr.Elem(), depth)
		return v
	}

	// Unwrap named types but keep error interface special
	switch t := typ.(type) {
	case *types.Named:
		// Check if it implements error interface
		errorType := types.Universe.Lookup("error").Type()
		if types.Implements(t, errorType.Underlying().(*types.Interface)) ||
			types.Implements(types.NewPointer(t), errorType.Underlying().(*types.Interface)) {
			return errors.New("test error")
		}
		return buildTestDataFromType(t.Underlying(), depth)

	case *types.Interface:
		// Check for error interface
		errorType := types.Universe.Lookup("error").Type()
		if types.Identical(t, errorType.Underlying()) {
			return errors.New("test error")
		}
		return "test"

	case *types.Basic:
		return zeroForBasic(t)

	case *types.Slice:
		elem := testValueForType(t.Elem(), depth+1)
		if elem == nil {
			return []any{}
		}
		if s, ok := elem.(string); ok {
			return []string{s}
		}
		return []any{elem}

	case *types.Struct:
		return buildTestDataFromType(t, depth)

	case *types.Map:
		return map[string]any{}
	}

	return "test"
}

func zeroForBasic(typ types.Type) any {
	basic, ok := typ.(*types.Basic)
	if !ok {
		return "test"
	}

	switch {
	case basic.Info()&types.IsString != 0:
		return "test"
	case basic.Info()&types.IsInteger != 0:
		return 42
	case basic.Info()&types.IsBoolean != 0:
		return true
	case basic.Info()&types.IsFloat != 0:
		return 3.14
	default:
		return "test"
	}
}

// resolveStringExpr extracts a string value from an AST expression.
// Handles basic literals and binary + (string concatenation).
func resolveStringExpr(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return stripQuotes(e.Value), true
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := resolveStringExpr(e.X)
		if !ok {
			return "", false
		}
		right, ok := resolveStringExpr(e.Y)
		if !ok {
			return "", false
		}
		return left + right, true
	default:
		return "", false
	}
}

// stripQuotes removes the surrounding quotes from a Go string literal,
// handling both interpreted ("...") and raw (`...`) strings.
func stripQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	if s[0] == '`' && s[len(s)-1] == '`' {
		return s[1 : len(s)-1]
	}
	if s[0] == '"' && s[len(s)-1] == '"' {
		// Handle escape sequences in interpreted strings
		// Use Go's own unquoting via a simple approach
		return unquoteInterpreted(s)
	}
	return s
}

// unquoteInterpreted handles basic Go string escape sequences.
func unquoteInterpreted(s string) string {
	// Strip outer quotes
	inner := s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		if inner[i] != '\\' {
			b.WriteByte(inner[i])
			continue
		}
		if i+1 >= len(inner) {
			b.WriteByte(inner[i])
			continue
		}
		i++
		switch inner[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		case '"':
			b.WriteByte('"')
		default:
			b.WriteByte('\\')
			b.WriteByte(inner[i])
		}
	}
	return b.String()
}
