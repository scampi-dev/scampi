// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"io/fs"
	"sort"
	"strings"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/std"
)

// ParamInfo describes one parameter of a stdlib function or decl.
type ParamInfo struct {
	Name       string
	Type       string
	Desc       string
	Default    string
	Required   bool
	Examples   []string
	EnumValues []string
}

// FuncInfo describes a function or decl from the standard library stubs.
type FuncInfo struct {
	Name       string
	Summary    string
	Params     []ParamInfo
	IsStep     bool
	ReturnType string // qualified return type, empty for funcs without one
}

// Catalog holds stdlib metadata, indexed for fast lookup during
// completion, hover, and signature help.
type Catalog struct {
	funcs     map[string]FuncInfo
	names     []string
	modules   map[string][]string // "posix" → ["copy", "dir", ...]
	byRetType map[string][]string // "posix.Source" → ["posix.source_local", ...]
}

func NewCatalog() *Catalog {
	c := &Catalog{
		funcs:     make(map[string]FuncInfo),
		modules:   make(map[string][]string),
		byRetType: make(map[string][]string),
	}
	c.loadFromStubs()
	c.loadTestStubs()
	c.buildIndex()
	return c
}

// Lookup returns the function with the given name, or false.
func (c *Catalog) Lookup(name string) (FuncInfo, bool) {
	f, ok := c.funcs[name]
	return f, ok
}

// Names returns all registered names in sorted order.
func (c *Catalog) Names() []string { return c.names }

// ModuleMembers returns the sub-function names for a dotted module
// (e.g. "posix" → ["copy", "dir", ...]).
func (c *Catalog) ModuleMembers(module string) []string {
	return c.modules[module]
}

// Modules returns the top-level module names.
func (c *Catalog) Modules() []string {
	out := make([]string, 0, len(c.modules))
	for k := range c.modules {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ByReturnType returns names of catalog entries whose return type matches
// the given qualified type (e.g. "posix.Source"). Used by completion to
// suggest constructors for typed kwargs.
func (c *Catalog) ByReturnType(typeName string) []string {
	return c.byRetType[typeName]
}

// Loading from stubs
// -----------------------------------------------------------------------------

// stubModule holds parsed metadata from a single .scampi stub file.
type stubModule struct {
	name  string
	enums map[string][]string     // enum name → variants
	types map[string][]*ast.Field // struct name → fields
	funcs []FuncInfo
	decls []FuncInfo
}

func (c *Catalog) loadFromStubs() {
	modules := parseAllStubs()

	// Collect enums across all modules for param EnumValues resolution.
	allEnums := make(map[string][]string)
	for _, mod := range modules {
		for name, variants := range mod.enums {
			allEnums[name] = variants
		}
	}

	// Register funcs and decls from each module.
	for _, mod := range modules {
		for _, f := range mod.funcs {
			name := qualifiedName(mod.name, f.Name)
			f.Name = name
			resolveEnumValues(f.Params, allEnums)
			c.funcs[name] = f
		}
		for _, d := range mod.decls {
			name := qualifiedName(mod.name, d.Name)
			d.Name = name
			resolveEnumValues(d.Params, allEnums)
			c.funcs[name] = d
		}

		// Register struct types as constructors (e.g. container.Healthcheck).
		for typeName, fields := range mod.types {
			name := qualifiedName(mod.name, typeName)
			params := fieldsToParams(fields, mod.name)
			resolveEnumValues(params, allEnums)
			c.funcs[name] = FuncInfo{Name: name, Params: params}
		}
	}
}

func qualifiedName(modName, name string) string {
	return modName + "." + name
}

func parseAllStubs() []*stubModule {
	var modules []*stubModule

	entries, err := fs.ReadDir(std.FS, ".")
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if e.IsDir() {
			// Check for submodule.
			subEntries, err := fs.ReadDir(std.FS, e.Name())
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if se.IsDir() || !strings.HasSuffix(se.Name(), ".scampi") {
					continue
				}
				path := e.Name() + "/" + se.Name()
				if mod := parseStubFile(path); mod != nil {
					modules = append(modules, mod)
				}
			}
		} else if strings.HasSuffix(e.Name(), ".scampi") {
			if mod := parseStubFile(e.Name()); mod != nil {
				modules = append(modules, mod)
			}
		}
	}
	return modules
}

func parseStubFile(path string) *stubModule {
	data, err := fs.ReadFile(std.FS, path)
	if err != nil {
		return nil
	}
	l := lex.New(path, data)
	p := parse.New(l)
	f := p.Parse()
	if f == nil {
		return nil
	}

	modName := "main"
	if f.Module != nil {
		modName = f.Module.Name.Name
	}

	mod := &stubModule{
		name:  modName,
		enums: make(map[string][]string),
		types: make(map[string][]*ast.Field),
	}

	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.EnumDecl:
			var variants []string
			for _, v := range d.Variants {
				variants = append(variants, v.Name)
			}
			mod.enums[d.Name.Name] = variants

		case *ast.TypeDecl:
			if d.Fields != nil {
				mod.types[d.Name.Name] = d.Fields
			}

		case *ast.FuncDecl:
			bf := funcDeclToInfo(d, modName)
			mod.funcs = append(mod.funcs, bf)

		case *ast.DeclDecl:
			bf := declDeclToInfo(d, modName)
			mod.decls = append(mod.decls, bf)
		}
	}

	return mod
}

func funcDeclToInfo(d *ast.FuncDecl, modName string) FuncInfo {
	params := fieldsToParams(d.Params, modName)
	return FuncInfo{
		Name:       d.Name.Name,
		Params:     params,
		ReturnType: qualifiedTypeString(d.Ret, modName),
	}
}

func declDeclToInfo(d *ast.DeclDecl, modName string) FuncInfo {
	params := fieldsToParams(d.Params, modName)
	retType := qualifiedTypeString(d.Ret, modName)
	return FuncInfo{
		Name:       declName(d),
		Params:     params,
		IsStep:     retTypeIsStep(d.Ret),
		ReturnType: retType,
	}
}

func declName(d *ast.DeclDecl) string {
	if len(d.Name.Parts) == 1 {
		return d.Name.Parts[0].Name
	}
	var parts []string
	for _, p := range d.Name.Parts {
		parts = append(parts, p.Name)
	}
	return strings.Join(parts, ".")
}

func retTypeIsStep(t ast.TypeExpr) bool {
	if t == nil {
		return false
	}
	nt, ok := t.(*ast.NamedType)
	if !ok {
		return false
	}
	// std.Step
	parts := nt.Name.Parts
	if len(parts) == 2 && parts[0].Name == "std" && parts[1].Name == "Step" {
		return true
	}
	if len(parts) == 1 && parts[0].Name == "Step" {
		return true
	}
	return false
}

func fieldsToParams(fields []*ast.Field, modName string) []ParamInfo {
	params := make([]ParamInfo, len(fields))
	for i, f := range fields {
		params[i] = ParamInfo{
			Name:     f.Name.Name,
			Type:     qualifiedTypeString(f.Type, modName),
			Required: f.Default == nil && !isOptionalType(f.Type),
		}
	}
	return params
}

func isOptionalType(t ast.TypeExpr) bool {
	_, ok := t.(*ast.OptionalType)
	return ok
}

// resolveEnumValues fills in EnumValues on params whose type matches
// a known enum. Tries both qualified (posix.PkgState) and bare (PkgState)
// names since stubs register enums by bare name.
func resolveEnumValues(params []ParamInfo, enums map[string][]string) {
	for i := range params {
		typeName := params[i].Type
		typeName = strings.TrimSuffix(typeName, "?")
		if variants, ok := enums[typeName]; ok {
			params[i].EnumValues = variants
			continue
		}
		// Try the leaf name after the dot (posix.PkgState → PkgState).
		if dot := strings.LastIndexByte(typeName, '.'); dot >= 0 {
			leaf := typeName[dot+1:]
			if variants, ok := enums[leaf]; ok {
				params[i].EnumValues = variants
			}
		}
	}
}

// qualifiedTypeString renders a type expression as a human-readable string,
// qualifying bare type names with the module they came from. Builtin types
// (string, int, bool, any) and already-dotted types (std.Step) are left as-is.
func qualifiedTypeString(t ast.TypeExpr, modName string) string {
	if t == nil {
		return ""
	}
	switch t := t.(type) {
	case *ast.NamedType:
		parts := t.Name.Parts
		if len(parts) == 1 {
			name := parts[0].Name
			if modName != "" && !isBuiltinType(name) {
				return modName + "." + name
			}
			return name
		}
		var names []string
		for _, p := range parts {
			names = append(names, p.Name)
		}
		return strings.Join(names, ".")
	case *ast.GenericType:
		var args []string
		for _, a := range t.Args {
			args = append(args, qualifiedTypeString(a, modName))
		}
		return t.Name.Name + "[" + strings.Join(args, ", ") + "]"
	case *ast.OptionalType:
		return qualifiedTypeString(t.Inner, modName) + "?"
	}
	return ""
}

// typeExprString renders a type expression without module qualification.
func typeExprString(t ast.TypeExpr) string {
	if t == nil {
		return ""
	}
	switch t := t.(type) {
	case *ast.NamedType:
		var parts []string
		for _, p := range t.Name.Parts {
			parts = append(parts, p.Name)
		}
		return strings.Join(parts, ".")
	case *ast.GenericType:
		var args []string
		for _, a := range t.Args {
			args = append(args, typeExprString(a))
		}
		return t.Name.Name + "[" + strings.Join(args, ", ") + "]"
	case *ast.OptionalType:
		return typeExprString(t.Inner) + "?"
	}
	return ""
}

var builtinTypes = map[string]bool{
	"string": true, "int": true, "bool": true, "any": true, "none": true,
}

func isBuiltinType(name string) bool {
	return builtinTypes[name]
}

// Test stubs (hardcoded until test framework has .scampi stubs)
// -----------------------------------------------------------------------------

func (c *Catalog) loadTestStubs() {
	for _, b := range testStubs() {
		c.funcs[b.Name] = b
	}
}

func testStubs() []FuncInfo {
	return []FuncInfo{
		{
			Name: "test.target.in_memory",
			Params: []ParamInfo{
				{Name: "name", Type: "string", Required: true},
				{Name: "files", Type: "map[string, string]"},
				{Name: "packages", Type: "list[string]"},
				{Name: "services", Type: "map[string, string]"},
				{Name: "dirs", Type: "list[string]"},
			},
		},
		{
			Name: "test.target.rest_mock",
			Params: []ParamInfo{
				{Name: "name", Type: "string", Required: true},
				{Name: "routes", Type: "map[string, any]"},
			},
		},
		{
			Name: "test.response",
			Params: []ParamInfo{
				{Name: "status", Type: "int", Required: true},
				{Name: "body", Type: "string"},
				{Name: "headers", Type: "map[string, string]"},
			},
		},
		{
			Name: "test.assert.that",
			Params: []ParamInfo{
				{Name: "target", Type: "test_target", Required: true},
			},
		},
	}
}

// Index building
// -----------------------------------------------------------------------------

func (c *Catalog) buildIndex() {
	c.names = make([]string, 0, len(c.funcs))
	for name := range c.funcs {
		c.names = append(c.names, name)
	}
	sort.Strings(c.names)

	// Build module membership from dotted names.
	for _, name := range c.names {
		parts := splitDot(name)
		if len(parts) < 2 {
			continue
		}
		mod := parts[0]
		member := name[len(mod)+1:] // everything after first dot
		c.modules[mod] = append(c.modules[mod], member)
	}

	// Build return-type index. Iterating in sorted order keeps the
	// completion list deterministic across runs.
	for _, name := range c.names {
		f := c.funcs[name]
		if f.ReturnType == "" {
			continue
		}
		c.byRetType[f.ReturnType] = append(c.byRetType[f.ReturnType], name)
	}
}

func splitDot(s string) []string {
	var parts []string
	start := 0
	for i := range len(s) {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
