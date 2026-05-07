// SPDX-License-Identifier: GPL-3.0-only

package eval

import (
	"io/fs"
	"strconv"
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/lang/token"
)

// EnvLookupFunc resolves an environment variable name to its value.
// The bool indicates whether the variable was set.
type EnvLookupFunc func(name string) (value string, found bool)

// BuiltinFunc is a caller-registered function dispatched by qualified
// stub name (e.g. "secrets.from_age") during eval. The eval layer
// invokes it when a bodyless stub func matches a registered name and
// forwards the call's positional and keyword arguments. All domain
// logic lives in the caller — the eval layer just passes values
// through.
//
// Builtins typically return an OpaqueVal wrapping runtime state that
// later pipeline stages (linker, attribute checks) can type-assert.
//
// Return ("", errMsg) to surface a diagnostic anchored at the call
// site's source span; the eval layer wraps it as a CodeCallError.
type BuiltinFunc func(positional []Value, kwargs map[string]Value) (Value, string)

// EmitCallback is invoked once per value the evaluator produces at
// top level or into a block body. It receives the value and a handle
// to the running evaluator so it can configure backends, register
// hooks, or inspect prior state.
type EmitCallback func(v Value, ev *Evaluator)

// Evaluator walks a type-checked AST and produces runtime values.
type Evaluator struct {
	env  *envScope
	errs []Error

	// Collected top-level values for the engine.
	result Result

	// stubFS is the stdlib stub filesystem for enum/type extraction.
	stubFS fs.FS

	// declReturns maps "module.decl" → return type name (e.g. "Step",
	// "Target"). Built from stubs at init time.
	declReturns map[string]string

	// returnVal is set by ReturnStmt to signal a return from any
	// nesting depth (if/for bodies). Checked after each statement
	// in callFunc and evalBlock loops.
	returnVal Value

	// envLookup resolves env vars. Injected by caller.
	envLookup EnvLookupFunc

	// lenient tells the evaluator to relax certain builtins that
	// would normally error on missing or unparseable input. Used by
	// analysis tools (e.g. the LSP) where inputs may be placeholders
	// rather than the real apply-time values. Apply-time eval must
	// never set this — runtime input errors are real bugs there.
	lenient bool

	// builtinFuncs are caller-registered functions dispatched by
	// qualified name (e.g. "secrets.from_age"). The eval layer
	// treats them as opaque — all domain logic lives in the caller.
	builtinFuncs map[string]BuiltinFunc

	// source holds the original source bytes for string extraction.
	source []byte

	// blockCollector collects values emitted as bare expressions
	// inside a block body (block[T] fill). nil when not inside a
	// block body.
	blockCollector *[]Value

	// onEmit is called for each emitted value (optional).
	onEmit EmitCallback

	// userModules holds parsed user module ASTs from scampi.mod deps.
	// Registered into the env at init time alongside std stubs.
	userModules []UserModule

	// siblingModules holds parsed ASTs from sibling files that share
	// the same module declaration. Their functions are registered
	// directly into the top-level env (bare names, no prefix).
	siblingModules []UserModule

	// modInternalMaps holds the full (pub + non-pub) symbol maps for
	// user modules. Used by callFunc for intra-module visibility so
	// non-pub helpers are callable within the same module. The env
	// only stores the pub-filtered map for external callers.
	modInternalMaps map[string]*MapVal

	// typeDefaults maps type name → AST fields for user-defined types
	// that have field defaults. Used by evalStructLit to fill in
	// omitted fields.
	typeDefaults map[string][]*ast.Field
}

// Error is an eval-time error.
type Error struct {
	Span token.Span
	Code errs.Code
	Msg  string
	Hint string
}

func (e Error) Error() string                { return e.Msg }
func (e Error) GetSpan() (start, end uint32) { return e.Span.Start, e.Span.End }
func (e Error) GetHint() string              { return e.Hint }
func (e Error) GetCode() errs.Code           { return e.Code }

// Option configures the evaluator.
type Option func(*Evaluator)

// WithEnv sets the environment variable resolver.
func WithEnv(fn EnvLookupFunc) Option {
	return func(e *Evaluator) { e.envLookup = fn }
}

// WithLenient enables tolerant evaluation: builtins that would
// normally error on missing or unparseable input return a benign
// default instead. Used by analysis tools (the LSP, future
// `scampi check --analyze`, etc.) where some inputs are placeholder
// values rather than the real apply-time ones. Apply-time eval
// should never set this — runtime input errors are real bugs there.
// See #264.
func WithLenient() Option {
	return func(e *Evaluator) { e.lenient = true }
}

// WithBuiltinFunc registers a named function dispatched by qualified
// name (e.g. "secrets.from_age") during eval. The eval layer has no
// knowledge of what the function does — all domain logic stays in
// the caller. Multiple calls with different names accumulate.
func WithBuiltinFunc(qualName string, fn BuiltinFunc) Option {
	return func(e *Evaluator) {
		if e.builtinFuncs == nil {
			e.builtinFuncs = map[string]BuiltinFunc{}
		}
		e.builtinFuncs[qualName] = fn
	}
}

// WithOnEmit registers a callback invoked whenever a value is emitted
// at top level or into a block body. The callback can inspect the value
// and set up state. This keeps the eval generic while allowing callers
// to react to domain-specific values.
func WithOnEmit(fn EmitCallback) Option {
	return func(e *Evaluator) { e.onEmit = fn }
}

// WithStubs sets the stdlib stub filesystem for enum and decl type
// resolution. Without this, the evaluator has no knowledge of module
// enums or builtin return types.
func WithStubs(fsys fs.FS) Option {
	return func(e *Evaluator) { e.stubFS = fsys }
}

// UserModule is a parsed user module ready for evaluation. Name is
// the module declaration name (e.g. "adguard"), File is the parsed
// AST, and Source is the raw bytes for string extraction.
type UserModule struct {
	Name   string
	File   *ast.File
	Source []byte
}

// WithUserModules registers external user modules in the evaluator
// so imported module decls/funcs can be resolved. Each module's
// declarations are registered under its name as a MapVal, and
// funcs/decls with bodies are registered as callable FuncVals so
// struct-lit expansion (e.g. adguard.dns_rewrite { ... }) works.
func WithUserModules(mods []UserModule) Option {
	return func(e *Evaluator) { e.userModules = mods }
}

// WithSiblingModules registers sibling module files (same `module`
// declaration, different file) directly into the top-level env so
// their functions are callable by bare name — the same-package model.
// Unlike WithUserModules (which namespaces under the module name),
// sibling functions are peers of the current file's own declarations.
func WithSiblingModules(mods []UserModule) Option {
	return func(e *Evaluator) { e.siblingModules = mods }
}

// Eval evaluates a type-checked AST file and returns the result.
func Eval(f *ast.File, source []byte, opts ...Option) (*Result, []Error) {
	ev := &Evaluator{
		env:    newEnv(nil),
		source: source,
	}
	for _, o := range opts {
		o(ev)
	}
	ev.registerStubInfo()
	ev.registerUserModules()
	ev.registerSiblingModules()
	ev.evalFile(f)
	ev.result.Bindings = ev.env.vars
	return &ev.result, ev.errs
}

// registerStubInfo parses the stub filesystem and populates the eval
// env with module enum maps and decl return type metadata.
func (ev *Evaluator) registerStubInfo() {
	if ev.stubFS == nil {
		ev.declReturns = map[string]string{}
		return
	}
	info := extractStubInfo(ev.stubFS)
	ev.declReturns = info.declReturns
	// Collect all module names from enums and funcs.
	allMods := map[string]bool{}
	for m := range info.enums {
		allMods[m] = true
	}
	for m := range info.funcs {
		allMods[m] = true
	}
	for modName := range allMods {
		modMap := &MapVal{}
		for enumName, variants := range info.enums[modName] {
			variantMap := &MapVal{}
			for _, v := range variants {
				variantMap.Set(v, &StringVal{V: v})
			}
			modMap.Set(enumName, variantMap)
		}
		// First pass: create FuncVals without scope.
		var bodied []*FuncVal
		for _, sf := range info.funcs[modName] {
			fv := &FuncVal{
				Name:     sf.Name,
				QualName: modName + "." + sf.Name,
				Params:   sf.Params,
				Defaults: sf.Defaults,
				RetType:  sf.RetType,
			}
			if sf.Body != nil {
				fv.body = sf.Body
				bodied = append(bodied, fv)
			}
			modMap.Set(sf.Name, fv)
		}
		ev.env.set(modName, modMap)
		// Second pass: set scope for bodied funcs to a child env
		// that includes the module's own functions as bare names
		// (same-module visibility).
		if len(bodied) > 0 {
			modScope := newEnv(ev.env)
			// Inject all module symbols (funcs, enums, types) as bare
			// names so same-module references work — including enum
			// defaults like Console.xtermjs.
			for i, k := range modMap.Keys {
				if sk, ok := k.(*StringVal); ok {
					modScope.set(sk.V, modMap.Values[i])
				}
			}
			for _, fv := range bodied {
				fv.scope = modScope
			}
		}
	}
}

// registerUserModules loads parsed user module ASTs into the eval
// env so that `module.func { ... }` invocations from imported modules
// can be expanded. Each module's funcs/decls are registered as
// FuncVals (with bodies for user-defined decls, without for stubs)
// under the module's declaration name in a MapVal.
func (ev *Evaluator) registerUserModules() {
	if ev.modInternalMaps == nil {
		ev.modInternalMaps = map[string]*MapVal{}
	}
	for _, um := range ev.userModules {
		fullMap := &MapVal{}
		pubMap := &MapVal{}
		for _, d := range um.File.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				retName := ""
				if d.Ret != nil {
					if _, isGeneric := d.Ret.(*ast.GenericType); isGeneric {
						retName = typeExprString(d.Ret)
					} else {
						retName = typeExprName(d.Ret)
					}
					ev.declReturns[um.Name+"."+d.Name.Name] = typeExprName(d.Ret)
				}
				var params []string
				var defaults []any
				for _, p := range d.Params {
					params = append(params, p.Name.Name)
					defaults = append(defaults, p.Default)
				}
				fv := &FuncVal{
					Name:     d.Name.Name,
					QualName: um.Name + "." + d.Name.Name,
					Params:   params,
					Defaults: defaults,
					RetType:  retName,
				}
				if d.Body != nil {
					fv.body = d.Body
					fv.scope = ev.env
				}
				fullMap.Set(d.Name.Name, fv)
				if d.Public {
					pubMap.Set(d.Name.Name, fv)
				}
			case *ast.DeclDecl:
				declName := d.Name.Parts[0].Name
				retName := ""
				if d.Ret != nil {
					retName = typeExprName(d.Ret)
					ev.declReturns[um.Name+"."+declName] = retName
				}
				var params []string
				var defaults []any
				for _, p := range d.Params {
					params = append(params, p.Name.Name)
					defaults = append(defaults, p.Default)
				}
				fv := &FuncVal{
					Name:     declName,
					QualName: um.Name + "." + declName,
					Params:   params,
					Defaults: defaults,
					RetType:  retName,
				}
				if d.Body != nil {
					fv.body = d.Body
					fv.scope = ev.env
				}
				fullMap.Set(declName, fv)
				if d.Public {
					pubMap.Set(declName, fv)
				}
			case *ast.TypeDecl:
				ev.registerTypeDefaults(um.Name+"."+d.Name.Name, d.Fields)
			case *ast.EnumDecl:
				variantMap := &MapVal{}
				for _, v := range d.Variants {
					variantMap.Set(v.Name, &StringVal{V: v.Name})
				}
				fullMap.Set(d.Name.Name, variantMap)
				if d.Public {
					pubMap.Set(d.Name.Name, variantMap)
				}
			}
		}
		ev.env.set(um.Name, pubMap)
		ev.modInternalMaps[um.Name] = fullMap

		// Second pass: top-level let-bindings. Each gets wrapped in a
		// ThunkVal so the cost of evaluation (incl. side effects like
		// `secrets.from_age` reading the keystore, or any apply-time
		// builtin fetches) is paid lazily — only when an importer
		// actually references the binding. Chains like
		//
		//   let _age = secrets.from_age(...)
		//   pub let admin_password = _age.get("...")
		//
		// work because the per-module sub-env (child of ev.env) holds
		// every module-local thunk; `_age` resolves from there when
		// `admin_password`'s thunk fires. See #269.
		moduleScope := newEnv(ev.env)
		for _, d := range um.File.Decls {
			let, ok := d.(*ast.LetDecl)
			if !ok {
				continue
			}
			expr := let.Value
			thunk := &ThunkVal{
				eval: func() Value {
					oldEnv := ev.env
					ev.env = moduleScope
					defer func() { ev.env = oldEnv }()
					return ev.evalExpr(expr)
				},
			}
			moduleScope.set(let.Name.Name, thunk)
			fullMap.Set(let.Name.Name, thunk)
			if let.Public {
				pubMap.Set(let.Name.Name, thunk)
			}
		}
	}
}

// registerSiblingModules injects functions from sibling files (same
// module, different file) directly into the top-level env. These are
// callable by bare name — same-package visibility.
func (ev *Evaluator) registerSiblingModules() {
	for _, um := range ev.siblingModules {
		for _, d := range um.File.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				var params []string
				var defaults []any
				for _, p := range d.Params {
					params = append(params, p.Name.Name)
					if p.Default != nil {
						defaults = append(defaults, p.Default)
					} else {
						defaults = append(defaults, nil)
					}
				}
				ev.env.set(d.Name.Name, &FuncVal{
					Name:     d.Name.Name,
					Params:   params,
					Defaults: defaults,
					body:     d.Body,
					scope:    ev.env,
				})
			case *ast.DeclDecl:
				if d.Body != nil {
					ev.env.set(d.Name.Parts[0].Name, &FuncVal{
						Name:  d.Name.Parts[0].Name,
						body:  d,
						scope: ev.env,
					})
				}
			case *ast.TypeDecl:
				ev.registerTypeDefaults(d.Name.Name, d.Fields)
			}
		}
	}
}

// stubFunc describes a function extracted from a .scampi stub file.
// For bodiless stubs, Body is nil. For funcs/decls with bodies,
// Body carries the AST node so the evaluator can execute it.
type stubFunc struct {
	Name     string
	Params   []string
	Defaults []any
	RetType  string
	Body     ast.Node
}

// stubInfo holds extracted metadata from parsed stub files.
type stubInfo struct {
	enums       map[string]map[string][]string // module → enum �� variants
	declReturns map[string]string              // "module.decl" → return type name
	funcs       map[string][]stubFunc          // module → func stubs
}

// extractStubInfo parses all .scampi files in the FS and returns enum
// declarations and decl return types.
func extractStubInfo(fsys fs.FS) stubInfo {
	info := stubInfo{
		enums:       map[string]map[string][]string{},
		declReturns: map[string]string{},
		funcs:       map[string][]stubFunc{},
	}
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".scampi") {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil
		}
		l := lex.New(path, data)
		p := parse.New(l)
		f := p.Parse()

		modName := ""
		if f.Module != nil {
			modName = f.Module.Name.Name
		}

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.EnumDecl:
				if info.enums[modName] == nil {
					info.enums[modName] = map[string][]string{}
				}
				var variants []string
				for _, v := range d.Variants {
					variants = append(variants, v.Name)
				}
				info.enums[modName][d.Name.Name] = variants
			case *ast.DeclDecl:
				declName := d.Name.Parts[0].Name
				retName := ""
				if d.Ret != nil {
					retName = typeExprName(d.Ret)
					info.declReturns[modName+"."+declName] = retName
				}
				var params []string
				var defaults []any
				for _, p := range d.Params {
					params = append(params, p.Name.Name)
					if p.Default != nil {
						defaults = append(defaults, p.Default)
					} else {
						defaults = append(defaults, nil)
					}
				}
				sf := stubFunc{
					Name:     declName,
					Params:   params,
					Defaults: defaults,
					RetType:  retName,
				}
				if d.Body != nil {
					sf.Body = d.Body
				}
				info.funcs[modName] = append(info.funcs[modName], sf)
			case *ast.FuncDecl:
				retName := ""
				if d.Ret != nil {
					if _, isGeneric := d.Ret.(*ast.GenericType); isGeneric {
						retName = typeExprString(d.Ret)
					} else {
						retName = typeExprName(d.Ret)
					}
					info.declReturns[modName+"."+d.Name.Name] = typeExprName(d.Ret)
				}
				var params []string
				var defaults []any
				for _, p := range d.Params {
					params = append(params, p.Name.Name)
					if p.Default != nil {
						defaults = append(defaults, p.Default)
					} else {
						defaults = append(defaults, nil)
					}
				}
				sf := stubFunc{
					Name:     d.Name.Name,
					Params:   params,
					Defaults: defaults,
					RetType:  retName,
				}
				if d.Body != nil {
					sf.Body = d.Body
				}
				info.funcs[modName] = append(info.funcs[modName], sf)
			}
		}
		return nil
	})
	return info
}

// typeExprString returns the full string representation of a type
// expression (e.g. "block[Deploy]", "string", "list[Step]").
func typeExprString(t ast.TypeExpr) string {
	switch t := t.(type) {
	case *ast.NamedType:
		parts := t.Name.Parts
		var names []string
		for _, p := range parts {
			names = append(names, p.Name)
		}
		return strings.Join(names, ".")
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

// typeExprName extracts the leaf type name from a type expression.
func typeExprName(t ast.TypeExpr) string {
	switch t := t.(type) {
	case *ast.NamedType:
		parts := t.Name.Parts
		return parts[len(parts)-1].Name
	}
	return ""
}

func (ev *Evaluator) errAt(span token.Span, code errs.Code, msg string) {
	ev.errs = append(ev.errs, Error{Span: span, Code: code, Msg: msg})
}

// evalFile evaluates a complete file.
func (ev *Evaluator) evalFile(f *ast.File) {
	// Register function and type declarations first (order-independent).
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			ev.evalDecl(d)
		case *ast.DeclDecl:
			ev.evalDecl(d)
		case *ast.TypeDecl:
			ev.registerTypeDefaults(d.Name.Name, d.Fields)
		case *ast.EnumDecl:
			// Enum declarations don't produce runtime values.
		}
	}

	// Evaluate let-bindings and statements in source order.
	// This ensures secrets() runs before secret() even when
	// secrets is a bare expression and secret is in a let.
	di, si := 0, 0
	for di < len(f.Decls) || si < len(f.Stmts) {
		// Pick whichever comes first in source.
		var declSpan, stmtSpan uint32
		if di < len(f.Decls) {
			declSpan = f.Decls[di].Span().Start
		}
		if si < len(f.Stmts) {
			stmtSpan = f.Stmts[si].Span().Start
		}

		if di < len(f.Decls) && (si >= len(f.Stmts) || declSpan <= stmtSpan) {
			d := f.Decls[di]
			di++
			if _, ok := d.(*ast.LetDecl); ok {
				ev.evalDecl(d)
			}
			// Skip func/decl/type/enum — already registered above.
		} else {
			ev.evalStmt(f.Stmts[si])
			si++
		}
	}
}

// Declarations
// -----------------------------------------------------------------------------

func (ev *Evaluator) evalDecl(d ast.Decl) {
	switch d := d.(type) {
	case *ast.LetDecl:
		v := ev.evalExpr(d.Value)
		ev.env.set(d.Name.Name, v)
	case *ast.FuncDecl:
		var params []string
		var defaults []any
		for _, p := range d.Params {
			params = append(params, p.Name.Name)
			if p.Default != nil {
				defaults = append(defaults, p.Default)
			} else {
				defaults = append(defaults, nil)
			}
		}
		ev.env.set(d.Name.Name, &FuncVal{
			Name:     d.Name.Name,
			Params:   params,
			Defaults: defaults,
			body:     d.Body,
			scope:    ev.env,
		})
	case *ast.DeclDecl:
		if d.Body != nil {
			ev.env.set(d.Name.Parts[0].Name, &FuncVal{
				Name:  d.Name.Parts[0].Name,
				body:  d,
				scope: ev.env,
			})
		}
	case *ast.TypeDecl:
		ev.registerTypeDefaults(d.Name.Name, d.Fields)
	case *ast.EnumDecl:
		// Enum declarations don't produce runtime values.
	}
}

// registerTypeDefaults records a type declaration's fields so that
// evalStructLit can fill in default values for omitted fields.
func (ev *Evaluator) registerTypeDefaults(name string, fields []*ast.Field) {
	if fields == nil {
		return // opaque type
	}
	hasDefaults := false
	for _, f := range fields {
		if f.Default != nil {
			hasDefaults = true
			break
		}
	}
	if !hasDefaults {
		return
	}
	if ev.typeDefaults == nil {
		ev.typeDefaults = map[string][]*ast.Field{}
	}
	ev.typeDefaults[name] = fields
}

// applyTypeDefaults fills in default values for fields missing from a
// struct literal. It checks both the leaf type name and the qualified
// name (for imported module types).
func (ev *Evaluator) applyTypeDefaults(typeName, qualName string, fields map[string]Value) {
	typeFields := ev.typeDefaults[typeName]
	if typeFields == nil && qualName != typeName {
		typeFields = ev.typeDefaults[qualName]
	}
	for _, f := range typeFields {
		if f.Default == nil {
			continue
		}
		if _, exists := fields[f.Name.Name]; exists {
			continue
		}
		fields[f.Name.Name] = ev.evalExpr(f.Default)
	}
}

// emitValue sends a value to the current collector. Inside a block
// body, values are collected into the block. At top level, values go
// into result.Exprs.
func (ev *Evaluator) emitValue(v Value) {
	if v == nil {
		return
	}
	if _, none := v.(*NoneVal); none {
		return
	}
	if ev.onEmit != nil {
		ev.onEmit(v, ev)
	}
	if ev.blockCollector != nil {
		*ev.blockCollector = append(*ev.blockCollector, v)
	} else {
		ev.result.Exprs = append(ev.result.Exprs, v)
	}
}

// Statements
// -----------------------------------------------------------------------------

func (ev *Evaluator) evalStmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.ExprStmt:
		v := ev.evalExpr(s.Expr)
		ev.emitValue(v)
	case *ast.LetStmt:
		v := ev.evalExpr(s.Decl.Value)
		ev.env.set(s.Decl.Name.Name, v)
	case *ast.ForStmt:
		ev.evalFor(s)
	case *ast.IfStmt:
		ev.evalIf(s)
	case *ast.ReturnStmt:
		if s.Value != nil {
			ev.returnVal = ev.evalExpr(s.Value)
		} else {
			ev.returnVal = &NoneVal{}
		}
	case *ast.AssignStmt:
		ev.evalAssign(s)
	}
}

func (ev *Evaluator) evalBlock(b *ast.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		ev.evalStmt(s)
		if ev.returnVal != nil {
			return
		}
	}
}

func (ev *Evaluator) evalFor(f *ast.ForStmt) {
	iter := ev.evalExpr(f.Iter)
	list, ok := iter.(*ListVal)
	if !ok {
		ev.errAt(f.SrcSpan, check.CodeForInRequiresList, "for-in requires a list")
		return
	}
	for _, item := range list.Items {
		child := newEnv(ev.env)
		child.set(f.Var.Name, item)
		prev := ev.env
		ev.env = child
		ev.evalBlock(f.Body)
		ev.env = prev
		if ev.returnVal != nil {
			return
		}
	}
}

func (ev *Evaluator) evalIf(s *ast.IfStmt) {
	cond := ev.evalExpr(s.Cond)
	if ev.asBool(cond, s.Cond.Span()) {
		child := newEnv(ev.env)
		prev := ev.env
		ev.env = child
		ev.evalBlock(s.Then)
		ev.env = prev
	} else if s.Else != nil {
		child := newEnv(ev.env)
		prev := ev.env
		ev.env = child
		ev.evalBlock(s.Else)
		ev.env = prev
	}
}

func (ev *Evaluator) evalAssign(a *ast.AssignStmt) {
	val := ev.evalExpr(a.Value)
	switch target := a.Target.(type) {
	case *ast.IndexExpr:
		coll := ev.evalExpr(target.X)
		key := ev.evalExpr(target.Index)
		if m, ok := coll.(*MapVal); ok {
			if sk, ok := key.(*StringVal); ok {
				m.Set(sk.V, val)
			}
		}
		if l, ok := coll.(*ListVal); ok {
			if ik, ok := key.(*IntVal); ok && int(ik.V) < len(l.Items) {
				l.Items[ik.V] = val
			}
		}
	case *ast.SelectorExpr:
		obj := ev.evalExpr(target.X)
		if sv, ok := obj.(*StructVal); ok {
			sv.Fields[target.Sel.Name] = val
		}
		if mv, ok := obj.(*MapVal); ok {
			mv.Set(target.Sel.Name, val)
		}
	}
}

// Expressions
// -----------------------------------------------------------------------------

func (ev *Evaluator) evalExpr(e ast.Expr) Value {
	if e == nil {
		return &NoneVal{}
	}
	switch e := e.(type) {
	case *ast.ParenExpr:
		return ev.evalExpr(e.Inner)
	case *ast.IntLit:
		v, _ := check.ParseInt(e.Raw)
		return &IntVal{V: v}
	case *ast.BoolLit:
		return &BoolVal{V: e.Value}
	case *ast.NoneLit:
		return &NoneVal{}
	case *ast.SelfLit:
		v, _ := ev.env.get("self")
		return v
	case *ast.StringLit:
		return ev.evalString(e)
	case *ast.Ident:
		v, ok := ev.env.get(e.Name)
		if !ok {
			ev.errAt(e.SrcSpan, check.CodeUndefined, "undefined: "+e.Name)
			return &NoneVal{}
		}
		// Force any ThunkVal (lazy `pub let` from a user module, #269)
		// at the access boundary, mirroring evalSelector / evalDottedName.
		// Without this, downstream consumers that switch on Value type
		// (linker → evalToGo for `any`-typed fields) silently turn
		// thunked lists into Go nil, breaking rest.resource drift
		// comparison. See the list-drift bug.
		return forceValue(v)
	case *ast.SelectorExpr:
		return ev.evalSelector(e)
	case *ast.BlockExpr:
		return ev.evalBlockExpr(e)
	case *ast.CallExpr:
		return ev.evalCall(e)
	case *ast.StructLit:
		return ev.evalStructLit(e)
	case *ast.ListLit:
		items := make([]Value, len(e.Items))
		for i, item := range e.Items {
			items[i] = ev.evalExpr(item)
		}
		return &ListVal{Items: items}
	case *ast.MapLit:
		m := &MapVal{}
		for _, entry := range e.Entries {
			m.Keys = append(m.Keys, ev.evalExpr(entry.Key))
			m.Values = append(m.Values, ev.evalExpr(entry.Value))
		}
		return m
	case *ast.IndexExpr:
		return ev.evalIndex(e)
	case *ast.BinaryExpr:
		return ev.evalBinary(e)
	case *ast.UnaryExpr:
		return ev.evalUnary(e)
	case *ast.IfExpr:
		cond := ev.evalExpr(e.Cond)
		if ev.asBool(cond, e.Cond.Span()) {
			return ev.evalExpr(e.Then)
		}
		return ev.evalExpr(e.Else)
	case *ast.ListComp:
		return ev.evalListComp(e)
	case *ast.DottedName:
		return ev.evalDottedName(e)
	}
	ev.errAt(e.Span(), check.CodeCannotEvaluate, "cannot evaluate expression")
	return &NoneVal{}
}

func (ev *Evaluator) evalString(s *ast.StringLit) Value {
	if len(s.Parts) == 1 {
		if t, ok := s.Parts[0].(*ast.StringText); ok {
			return &StringVal{V: resolveEscapes(t.Raw)}
		}
	}
	var buf []byte
	for _, p := range s.Parts {
		switch p := p.(type) {
		case *ast.StringText:
			buf = append(buf, resolveEscapes(p.Raw)...)
		case *ast.StringInterp:
			v := ev.evalExpr(p.Expr)
			buf = append(buf, valueToString(v)...)
		}
	}
	return &StringVal{V: string(buf)}
}

func (ev *Evaluator) evalSelector(sel *ast.SelectorExpr) Value {
	// Force the receiver: when sel.X is an Ident whose env binding is
	// a ThunkVal (a `pub let` in a user module — see #269), the
	// selector access has to drive the thunk before peering into a
	// concrete StructVal/MapVal.
	x := forceValue(ev.evalExpr(sel.X))
	name := sel.Sel.Name
	if sv, ok := x.(*StructVal); ok {
		if v, exists := sv.Fields[name]; exists {
			return forceValue(v)
		}
	}
	if mv, ok := x.(*MapVal); ok {
		if v, exists := mv.Get(name); exists {
			return forceValue(v)
		}
	}
	ev.errAt(sel.SrcSpan, check.CodeCannotAccess, "cannot access ."+name)
	return &NoneVal{}
}

func (ev *Evaluator) evalDottedName(dn *ast.DottedName) Value {
	if len(dn.Parts) == 0 {
		return &NoneVal{}
	}
	v, ok := ev.env.get(dn.Parts[0].Name)
	if !ok {
		ev.errAt(dn.Parts[0].SrcSpan, check.CodeUndefined, "undefined: "+dn.Parts[0].Name)
		return &NoneVal{}
	}
	v = forceValue(v)
	for _, part := range dn.Parts[1:] {
		if sv, ok := v.(*StructVal); ok {
			v = forceValue(sv.Fields[part.Name])
			continue
		}
		if mv, ok := v.(*MapVal); ok {
			got, _ := mv.Get(part.Name)
			v = forceValue(got)
			continue
		}
		ev.errAt(part.SrcSpan, check.CodeCannotAccess, "cannot access ."+part.Name)
		return &NoneVal{}
	}
	return v
}

func (ev *Evaluator) evalCall(call *ast.CallExpr) Value {
	// UFCS dispatch: when the type checker has marked this call
	// as `x.f(args)` ≡ `f(x, args)`, evaluate the receiver from
	// `call.Fn.(*ast.SelectorExpr).X`, look up `f` (either at
	// top-level env when UFCSModule is empty, or via the imported
	// module's MapVal when set), and prepend the receiver as the
	// first positional arg. The AST shape stays as a SelectorExpr
	// call so source spans and tooling are unaffected.
	var fv *FuncVal
	var leadingArgs []Value
	if call.UFCS {
		sel := call.Fn.(*ast.SelectorExpr)
		recv := forceValue(ev.evalExpr(sel.X))
		fn, errMsg := ev.lookupUFCSFunc(call.UFCSModule, sel.Sel.Name)
		if errMsg != "" {
			ev.errAt(call.SrcSpan, check.CodeCallError, errMsg)
			return &NoneVal{}
		}
		fv = fn
		leadingArgs = []Value{recv}
	} else {
		fn := ev.evalExpr(call.Fn)
		v, ok := fn.(*FuncVal)
		if !ok {
			ev.errAt(call.SrcSpan, check.CodeCannotCall, "cannot call non-function")
			return &NoneVal{}
		}
		fv = v
	}

	argMap := make(map[string]Value, len(call.Args))
	positional := leadingArgs
	for _, a := range call.Args {
		// Force at the call boundary — args derived from a thunked
		// `pub let` (#269) must materialise before reaching builtin
		// or user-defined receivers that type-assert on Value kind.
		v := forceValue(ev.evalExpr(a.Value))
		if a.Name != nil {
			argMap[a.Name.Name] = v
		} else {
			positional = append(positional, v)
		}
	}
	return ev.callFunc(fv, positional, argMap, call.SrcSpan)
}

// lookupUFCSFunc resolves a UFCS callee. When module is empty, the
// function name is looked up in the top-level env (the file's
// declared funcs). When module is set, the lookup goes through the
// module's MapVal and pulls the function out by name. Returns an
// error string instead of panicking so the caller can attach the
// call's source span to the diagnostic.
func (ev *Evaluator) lookupUFCSFunc(module, name string) (*FuncVal, string) {
	if module == "" {
		v, ok := ev.env.get(name)
		if !ok {
			return nil, "ufcs: undefined " + name
		}
		fv, ok := v.(*FuncVal)
		if !ok {
			return nil, "ufcs: " + name + " is not a function"
		}
		return fv, ""
	}
	modVal, ok := ev.env.get(module)
	if !ok {
		return nil, "ufcs: module " + module + " not in scope"
	}
	modMap, ok := modVal.(*MapVal)
	if !ok {
		return nil, "ufcs: " + module + " is not a module"
	}
	member, ok := modMap.Get(name)
	if !ok {
		return nil, "ufcs: " + module + "." + name + " not found"
	}
	fv, ok := member.(*FuncVal)
	if !ok {
		return nil, "ufcs: " + module + "." + name + " is not a function"
	}
	return fv, ""
}

func (ev *Evaluator) evalBlockExpr(e *ast.BlockExpr) Value {
	target := ev.evalExpr(e.Target)
	bv, ok := target.(*BlockVal)
	if !ok {
		ev.errAt(e.SrcSpan, check.CodeCannotFillBlock, "cannot fill non-block value")
		return &NoneVal{}
	}
	return ev.fillBlock(bv, e.Body)
}

func (ev *Evaluator) fillBlock(bv *BlockVal, body *ast.Block) Value {
	var collected []Value
	prevCollector := ev.blockCollector
	ev.blockCollector = &collected
	childEnv := newEnv(ev.env)
	prevEnv := ev.env
	ev.env = childEnv
	if body != nil {
		ev.evalBlock(body)
	}
	ev.env = prevEnv
	ev.blockCollector = prevCollector
	return &BlockResultVal{
		TypeName: bv.InnerType,
		FuncName: bv.FuncName,
		Fields:   bv.Fields,
		Body:     collected,
	}
}

// fillBlockFromStmts is like fillBlock but takes raw statements
// (from a StructLit body, used for ident { stmts } syntax).
func (ev *Evaluator) fillBlockFromStmts(bv *BlockVal, stmts []ast.Stmt) Value {
	var collected []Value
	prevCollector := ev.blockCollector
	ev.blockCollector = &collected
	childEnv := newEnv(ev.env)
	prevEnv := ev.env
	ev.env = childEnv
	for _, s := range stmts {
		ev.evalStmt(s)
	}
	ev.env = prevEnv
	ev.blockCollector = prevCollector
	return &BlockResultVal{
		TypeName: bv.InnerType,
		FuncName: bv.FuncName,
		Fields:   bv.Fields,
		Body:     collected,
	}
}

// AddError reports an error from an onEmit callback.
func (ev *Evaluator) AddError(code errs.Code, msg string) {
	ev.errAt(token.Span{}, code, msg)
}

func (ev *Evaluator) callRange(positional []Value, kwargs map[string]Value) Value {
	n := int64(0)
	if len(positional) > 0 {
		if iv, ok := positional[0].(*IntVal); ok {
			n = iv.V
		}
	}
	if nv, ok := kwargs["n"]; ok {
		if iv, ok := nv.(*IntVal); ok {
			n = iv.V
		}
	}
	items := make([]Value, n)
	for i := int64(0); i < n; i++ {
		items[i] = &IntVal{V: i}
	}
	return &ListVal{Items: items}
}

func (ev *Evaluator) callTrimPrefix(positional []Value, kwargs map[string]Value) Value {
	s := stringArg(positional, kwargs, "s", 0)
	prefix := stringArg(positional, kwargs, "prefix", 1)
	return &StringVal{V: strings.TrimPrefix(s, prefix)}
}

func (ev *Evaluator) callTrimSuffix(positional []Value, kwargs map[string]Value) Value {
	s := stringArg(positional, kwargs, "s", 0)
	suffix := stringArg(positional, kwargs, "suffix", 1)
	return &StringVal{V: strings.TrimSuffix(s, suffix)}
}

func (ev *Evaluator) callParseInt(positional []Value, kwargs map[string]Value, span token.Span) Value {
	s := stringArg(positional, kwargs, "s", 0)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if ev.lenient {
			// Analysis tools (e.g. LSP) may feed placeholder strings
			// derived from unset env vars. Returning 0 silently
			// keeps downstream evaluation going without cascade
			// diagnostics. See #264.
			return &IntVal{V: 0}
		}
		ev.errAt(span, check.CodeCallError, "parse_int: cannot parse "+strconv.Quote(s)+" as int")
		return &IntVal{V: 0}
	}
	return &IntVal{V: n}
}

func stringArg(positional []Value, kwargs map[string]Value, name string, idx int) string {
	if idx < len(positional) {
		if sv, ok := positional[idx].(*StringVal); ok {
			return sv.V
		}
	}
	if v, ok := kwargs[name]; ok {
		if sv, ok := v.(*StringVal); ok {
			return sv.V
		}
	}
	return ""
}

func (ev *Evaluator) callRef(positional []Value, kwargs map[string]Value) Value {
	var step *StructVal
	var expr string

	if len(positional) > 0 {
		step, _ = positional[0].(*StructVal)
	}
	if len(positional) > 1 {
		if s, ok := positional[1].(*StringVal); ok {
			expr = s.V
		}
	}
	if sv, ok := kwargs["step"]; ok {
		step, _ = sv.(*StructVal)
	}
	if sv, ok := kwargs["expr"]; ok {
		if s, ok := sv.(*StringVal); ok {
			expr = s.V
		}
	}

	if step == nil {
		return &NoneVal{}
	}
	return &RefVal{Step: step, Expr: expr}
}

func (ev *Evaluator) callEnv(positional []Value, kwargs map[string]Value, span token.Span) Value {
	name := ""
	if len(positional) > 0 {
		if s, ok := positional[0].(*StringVal); ok {
			name = s.V
		}
	}
	if n, ok := kwargs["name"]; ok {
		if s, ok := n.(*StringVal); ok {
			name = s.V
		}
	}
	if ev.envLookup != nil {
		if v, ok := ev.envLookup(name); ok {
			return &StringVal{V: v}
		}
	}
	// Check for default.
	def := ""
	if len(positional) > 1 {
		if s, ok := positional[1].(*StringVal); ok {
			def = s.V
		}
	}
	if d, ok := kwargs["default"]; ok {
		if s, ok := d.(*StringVal); ok {
			def = s.V
		}
	}
	if def != "" {
		return &StringVal{V: def}
	}
	ev.errAt(span, check.CodeEnvVarNotSet, "required environment variable \""+name+"\" is not set")
	return &StringVal{V: ""}
}

func (ev *Evaluator) callFunc(fv *FuncVal, positional []Value, kwargs map[string]Value, callSpan token.Span) Value {
	// Stub func (no body) — produce value based on return type.
	if fv.body == nil && fv.RetType != "" {
		fields := make(map[string]Value, len(fv.Params))
		for i, name := range fv.Params {
			if v, ok := kwargs[name]; ok {
				fields[name] = v
			} else if i < len(positional) {
				fields[name] = positional[i]
			}
		}

		// Caller-registered builtins (keyed by qualified name).
		if fn, ok := ev.builtinFuncs[fv.QualName]; ok {
			result, errMsg := fn(positional, kwargs)
			if errMsg != "" {
				ev.errAt(callSpan, check.CodeCallError, errMsg)
				return &NoneVal{}
			}
			return result
		}

		// Scalar builtins with runtime callbacks.
		switch fv.Name {
		case "env":
			return ev.callEnv(positional, kwargs, callSpan)
		case "range":
			return ev.callRange(positional, kwargs)
		case "ref":
			return ev.callRef(positional, kwargs)
		case "trim_prefix":
			return ev.callTrimPrefix(positional, kwargs)
		case "trim_suffix":
			return ev.callTrimSuffix(positional, kwargs)
		case "parse_int":
			return ev.callParseInt(positional, kwargs, callSpan)
		}

		if strings.HasPrefix(fv.RetType, "block[") && strings.HasSuffix(fv.RetType, "]") {
			innerType := fv.RetType[6 : len(fv.RetType)-1]
			return &BlockVal{FuncName: fv.Name, InnerType: innerType, Fields: fields}
		}
		return &StructVal{
			TypeName: fv.Name,
			QualName: fv.QualName,
			RetType:  fv.RetType,
			Fields:   fields,
		}
	}

	body, ok := fv.body.(*ast.Block)
	if !ok {
		return &NoneVal{}
	}
	parent, _ := fv.scope.(*envScope)
	child := newEnv(parent)
	// If this is a user module function (QualName has a module
	// prefix), bind the module's sibling functions into the body
	// scope so bare references like `get_nginx_proxy_hosts()` work
	// within the module — same as Go's intra-package visibility.
	// Uses modInternalMaps (full pub+non-pub set) rather than the
	// env map (pub-only) so non-pub helpers are reachable.
	if dot := strings.IndexByte(fv.QualName, '.'); dot > 0 {
		modPrefix := fv.QualName[:dot]
		if mv, ok := ev.modInternalMaps[modPrefix]; ok {
			for i, k := range mv.Keys {
				if sk, ok := k.(*StringVal); ok {
					child.set(sk.V, mv.Values[i])
				}
			}
		}
	}
	prev := ev.env
	ev.env = child
	for i, name := range fv.Params {
		if v, ok := kwargs[name]; ok {
			child.set(name, v)
		} else if i < len(positional) {
			child.set(name, positional[i])
		} else if i < len(fv.Defaults) && fv.Defaults[i] != nil {
			if defExpr, ok := fv.Defaults[i].(ast.Expr); ok {
				child.set(name, ev.evalExpr(defExpr))
			} else {
				child.set(name, &NoneVal{})
			}
		} else {
			child.set(name, &NoneVal{})
		}
	}
	ev.returnVal = nil
	for _, s := range body.Stmts {
		ev.evalStmt(s)
		if ev.returnVal != nil {
			break
		}
	}
	retVal := ev.returnVal
	ev.returnVal = nil
	ev.env = prev
	if retVal == nil {
		return &NoneVal{}
	}
	return retVal
}

func (ev *Evaluator) evalStructLit(lit *ast.StructLit) Value {
	fields := make(map[string]Value, len(lit.Fields))
	fieldSpans := make(map[string]token.Span, len(lit.Fields))
	for _, f := range lit.Fields {
		fields[f.Name.Name] = ev.evalExpr(f.Value)
		fieldSpans[f.Name.Name] = f.Value.Span()
	}

	typeName := structLitTypeName(lit)
	qualName := structLitQualifiedName(lit)
	retType := ev.declReturns[qualName]

	// User-defined decl with body: expand with self bound.
	// Try the leaf name first (same-module decls), then the
	// qualified name (imported module decls like adguard.dns_rewrite).
	if fv, ok := ev.lookupStep(typeName); ok {
		return ev.expandUserStep(fv, fields)
	}
	if qualName != typeName {
		if fv, ok := ev.lookupStepQualified(qualName); ok && fv.body != nil {
			// User module decls with bodies are evaluated via
			// callFunc which handles return statements and builds
			// proper StructVals with declReturns resolution. Stubs
			// (no body) fall through to the normal struct-lit path.
			//
			// Return the result directly — don't emitValue here.
			// The caller handles emission: ExprStmt emits via
			// evalStmt; LetStmt binds without emitting so the user
			// can let-bind and re-emit later (ref() pattern).
			return ev.callFunc(fv, nil, fields, lit.Span())
		}
	}

	// Block fill on a let-bound block value (ident { stmts }).
	if retType == "" && len(lit.Body) > 0 {
		v, ok := ev.env.get(typeName)
		if ok {
			if bv, ok := v.(*BlockVal); ok {
				return ev.fillBlockFromStmts(bv, lit.Body)
			}
		}
	}

	// Fill defaults from the type declaration for any missing fields.
	ev.applyTypeDefaults(typeName, qualName, fields)

	return &StructVal{
		TypeName:   typeName,
		QualName:   qualName,
		RetType:    retType,
		Fields:     fields,
		SrcSpan:    lit.SrcSpan,
		FieldSpans: fieldSpans,
	}
}

func (ev *Evaluator) lookupStep(name string) (*FuncVal, bool) {
	v, ok := ev.env.get(name)
	if !ok {
		return nil, false
	}
	fv, ok := v.(*FuncVal)
	if !ok {
		return nil, false
	}
	return fv, true
}

// lookupStepQualified resolves "module.func" by walking into the
// module's MapVal. Used for imported user-defined decls like
// adguard.dns_rewrite.
func (ev *Evaluator) lookupStepQualified(qualName string) (*FuncVal, bool) {
	dot := strings.IndexByte(qualName, '.')
	if dot < 0 {
		return nil, false
	}
	modName := qualName[:dot]
	funcName := qualName[dot+1:]
	modVal, ok := ev.env.get(modName)
	if !ok {
		return nil, false
	}
	mv, ok := modVal.(*MapVal)
	if !ok {
		return nil, false
	}
	v, ok := mv.Get(funcName)
	if !ok {
		return nil, false
	}
	fv, ok := v.(*FuncVal)
	if !ok {
		return nil, false
	}
	return fv, true
}

// expandUserStep evaluates a user-defined step body with self bound to
// the provided fields. All StepVals emitted by the body are
// collected and returned. If inside a deploy, they're also appended to
// the current deploy's steps.
func (ev *Evaluator) expandUserStep(fv *FuncVal, fields map[string]Value) Value {
	stepDecl, ok := fv.body.(*ast.DeclDecl)
	if !ok || stepDecl.Body == nil {
		return &StructVal{TypeName: fv.Name, Fields: fields}
	}
	selfVal := &StructVal{TypeName: "self", Fields: fields}
	child := newEnv(ev.env)
	child.set("self", selfVal)
	// Also bind param names so self.member works AND member works.
	for _, p := range stepDecl.Params {
		if v, exists := fields[p.Name.Name]; exists {
			child.set(p.Name.Name, v)
		}
	}
	prevEnv := ev.env
	ev.env = child
	// Walk the body. If we encounter a return statement, evaluate
	// its value and emit it as a step — user module decls like
	// `decl dns_rewrite(...) { return rest.resource { ... } }` use
	// return to produce their step value. Non-return statements
	// (let bindings, if/for, bare expression steps) are handled
	// by evalStmt/emitValue as usual.
	//
	// Returns nested in if/for set ev.returnVal via evalStmt; check
	// it after each statement so conditional returns aren't dropped
	// (matches the callFunc body loop).
	prevReturn := ev.returnVal
	ev.returnVal = nil
	for _, s := range stepDecl.Body.Stmts {
		if rs, ok := s.(*ast.ReturnStmt); ok && rs.Value != nil {
			v := ev.evalExpr(rs.Value)
			ev.emitValue(v)
			ev.returnVal = nil
			ev.env = prevEnv
			ev.returnVal = prevReturn
			return &NoneVal{}
		}
		ev.evalStmt(s)
		if ev.returnVal != nil {
			ev.emitValue(ev.returnVal)
			ev.returnVal = nil
			break
		}
	}
	ev.env = prevEnv
	ev.returnVal = prevReturn
	return &NoneVal{}
}

func (ev *Evaluator) evalIndex(idx *ast.IndexExpr) Value {
	coll := ev.evalExpr(idx.X)
	key := ev.evalExpr(idx.Index)
	switch c := coll.(type) {
	case *ListVal:
		if ik, ok := key.(*IntVal); ok && int(ik.V) < len(c.Items) {
			return c.Items[ik.V]
		}
	case *MapVal:
		if sk, ok := key.(*StringVal); ok {
			if v, found := c.Get(sk.V); found {
				return v
			}
		}
	case *StructVal:
		if sk, ok := key.(*StringVal); ok {
			if v, found := c.Fields[sk.V]; found {
				return v
			}
		}
	}
	return &NoneVal{}
}

func (ev *Evaluator) evalBinary(bin *ast.BinaryExpr) Value {
	lv := ev.evalExpr(bin.Left)
	rv := ev.evalExpr(bin.Right)

	switch bin.Op {
	case token.Plus:
		if ls, ok := lv.(*StringVal); ok {
			if rs, ok := rv.(*StringVal); ok {
				return &StringVal{V: ls.V + rs.V}
			}
		}
		if li, ok := lv.(*IntVal); ok {
			if ri, ok := rv.(*IntVal); ok {
				return &IntVal{V: li.V + ri.V}
			}
		}
		if ll, ok := lv.(*ListVal); ok {
			if rl, ok := rv.(*ListVal); ok {
				items := make([]Value, 0, len(ll.Items)+len(rl.Items))
				items = append(items, ll.Items...)
				items = append(items, rl.Items...)
				return &ListVal{Items: items}
			}
		}
	case token.Minus:
		return intBinOp(lv, rv, func(a, b int64) int64 { return a - b })
	case token.Star:
		return intBinOp(lv, rv, func(a, b int64) int64 { return a * b })
	case token.Slash:
		if isIntZero(rv) {
			ev.errAt(bin.SrcSpan, check.CodeDivByZero, "division by zero")
			return &NoneVal{}
		}
		return intBinOp(lv, rv, func(a, b int64) int64 { return a / b })
	case token.Percent:
		if isIntZero(rv) {
			ev.errAt(bin.SrcSpan, check.CodeDivByZero, "modulo by zero")
			return &NoneVal{}
		}
		return intBinOp(lv, rv, func(a, b int64) int64 { return a % b })
	case token.Eq:
		return &BoolVal{V: valuesEqual(lv, rv)}
	case token.Neq:
		return &BoolVal{V: !valuesEqual(lv, rv)}
	case token.Lt:
		return &BoolVal{V: compareInts(lv, rv) < 0}
	case token.Gt:
		return &BoolVal{V: compareInts(lv, rv) > 0}
	case token.Leq:
		return &BoolVal{V: compareInts(lv, rv) <= 0}
	case token.Geq:
		return &BoolVal{V: compareInts(lv, rv) >= 0}
	case token.And:
		return &BoolVal{V: ev.asBool(lv, bin.Left.Span()) && ev.asBool(rv, bin.Right.Span())}
	case token.Or:
		return &BoolVal{V: ev.asBool(lv, bin.Left.Span()) || ev.asBool(rv, bin.Right.Span())}
	case token.In:
		return &BoolVal{V: valueIn(lv, rv)}
	}
	return &NoneVal{}
}

func (ev *Evaluator) evalUnary(un *ast.UnaryExpr) Value {
	xv := ev.evalExpr(un.X)
	switch un.Op {
	case token.Not:
		return &BoolVal{V: !ev.asBool(xv, un.X.Span())}
	case token.Minus:
		if iv, ok := xv.(*IntVal); ok {
			return &IntVal{V: -iv.V}
		}
	}
	return &NoneVal{}
}

func (ev *Evaluator) evalListComp(comp *ast.ListComp) Value {
	iter := ev.evalExpr(comp.Iter)
	list, ok := iter.(*ListVal)
	if !ok {
		return &ListVal{}
	}
	var items []Value
	for _, item := range list.Items {
		child := newEnv(ev.env)
		child.set(comp.Var.Name, item)
		prev := ev.env
		ev.env = child
		if comp.Cond != nil {
			cond := ev.evalExpr(comp.Cond)
			if !ev.asBool(cond, comp.Cond.Span()) {
				ev.env = prev
				continue
			}
		}
		items = append(items, ev.evalExpr(comp.Expr))
		ev.env = prev
	}
	return &ListVal{Items: items}
}

// Helpers
// -----------------------------------------------------------------------------

// asBool unwraps a BoolVal at the given span. The type checker
// rejects most non-bool uses, but optional-typed values (`T?`) can
// be `none` at runtime — emit a typed diagnostic and return false
// instead of panicking.
func (ev *Evaluator) asBool(v Value, span token.Span) bool {
	if bv, ok := v.(*BoolVal); ok {
		return bv.V
	}
	ev.errAt(span, check.CodeNotBool, "expected bool, got "+typeNameOf(v))
	return false
}

// isIntZero reports whether v is an IntVal with value 0.
func isIntZero(v Value) bool {
	iv, ok := v.(*IntVal)
	return ok && iv.V == 0
}

// typeNameOf returns a short user-facing type name for diagnostics.
func typeNameOf(v Value) string {
	switch v.(type) {
	case *BoolVal:
		return "bool"
	case *IntVal:
		return "int"
	case *StringVal:
		return "string"
	case *NoneVal:
		return "none"
	case *ListVal:
		return "list"
	case *MapVal:
		return "map"
	case *StructVal:
		return "struct"
	case *FuncVal:
		return "func"
	case *BlockVal, *BlockResultVal:
		return "block"
	case *OpaqueVal:
		return "opaque"
	case *RefVal:
		return "ref"
	default:
		return "unknown"
	}
}

func valueToString(v Value) string {
	if v == nil {
		return "none"
	}
	switch v := v.(type) {
	case *StringVal:
		return v.V
	case *IntVal:
		return v.String()
	case *BoolVal:
		return v.String()
	case *NoneVal:
		return "none"
	}
	return v.String()
}

func valuesEqual(a, b Value) bool {
	switch av := a.(type) {
	case *StringVal:
		if bv, ok := b.(*StringVal); ok {
			return av.V == bv.V
		}
	case *IntVal:
		if bv, ok := b.(*IntVal); ok {
			return av.V == bv.V
		}
	case *BoolVal:
		if bv, ok := b.(*BoolVal); ok {
			return av.V == bv.V
		}
	case *NoneVal:
		_, ok := b.(*NoneVal)
		return ok
	}
	return false
}

func compareInts(a, b Value) int {
	ai, aok := a.(*IntVal)
	bi, bok := b.(*IntVal)
	if !aok || !bok {
		return 0
	}
	switch {
	case ai.V < bi.V:
		return -1
	case ai.V > bi.V:
		return 1
	}
	return 0
}

func intBinOp(a, b Value, op func(int64, int64) int64) Value {
	ai, aok := a.(*IntVal)
	bi, bok := b.(*IntVal)
	if !aok || !bok {
		return &NoneVal{}
	}
	return &IntVal{V: op(ai.V, bi.V)}
}

func valueIn(needle Value, haystack Value) bool {
	switch h := haystack.(type) {
	case *ListVal:
		for _, item := range h.Items {
			if valuesEqual(needle, item) {
				return true
			}
		}
	case *MapVal:
		if sk, ok := needle.(*StringVal); ok {
			_, found := h.Get(sk.V)
			return found
		}
	}
	return false
}

func resolveEscapes(raw string) string {
	if len(raw) == 0 {
		return raw
	}
	buf := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			switch raw[i+1] {
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			case 'r':
				buf = append(buf, '\r')
			case '\\':
				buf = append(buf, '\\')
			case '"':
				buf = append(buf, '"')
			case '$':
				buf = append(buf, '$')
			case '0':
				buf = append(buf, 0)
			default:
				buf = append(buf, raw[i+1])
			}
			i++
			continue
		}
		buf = append(buf, raw[i])
	}
	return string(buf)
}

func structLitTypeName(lit *ast.StructLit) string {
	if lit.Type == nil {
		return ""
	}
	nt, ok := lit.Type.(*ast.NamedType)
	if !ok {
		return ""
	}
	if len(nt.Name.Parts) == 0 {
		return ""
	}
	return nt.Name.Parts[len(nt.Name.Parts)-1].Name
}

// structLitQualifiedName returns "module.decl" for a struct lit like
// posix.copy { ... } → "posix.copy". For unqualified names like
// User { ... } → "User".
func structLitQualifiedName(lit *ast.StructLit) string {
	if lit.Type == nil {
		return ""
	}
	nt, ok := lit.Type.(*ast.NamedType)
	if !ok {
		return ""
	}
	parts := nt.Name.Parts
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0].Name
	}
	return parts[0].Name + "." + parts[1].Name
}
