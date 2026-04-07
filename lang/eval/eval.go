// SPDX-License-Identifier: GPL-3.0-only

package eval

import (
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/token"
)

// Evaluator walks a type-checked AST and produces runtime values.
type Evaluator struct {
	env  *envScope
	errs []Error

	// Collected top-level values for the engine.
	result Result

	// envLookup resolves env vars. Injected by caller.
	envLookup func(string) (string, bool)

	// secretLookup resolves secrets. Injected by caller.
	secretLookup func(string) (string, error)

	// source holds the original source bytes for string extraction.
	source []byte

	// stepEmit is called when a bare step invocation is encountered
	// inside a deploy body. It appends to the current deploy's steps.
	currentDeploy *DeployVal
}

// Error is an eval-time error.
type Error struct {
	Span token.Span
	Msg  string
}

func (e Error) Error() string { return e.Msg }

// Option configures the evaluator.
type Option func(*Evaluator)

// WithEnv sets the environment variable resolver.
func WithEnv(fn func(string) (string, bool)) Option {
	return func(e *Evaluator) { e.envLookup = fn }
}

// WithSecrets sets the secret resolver.
func WithSecrets(fn func(string) (string, error)) Option {
	return func(e *Evaluator) { e.secretLookup = fn }
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
	ev.registerStdEnums()
	ev.evalFile(f)
	ev.result.Bindings = ev.env.vars
	return &ev.result, ev.errs
}

// registerStdEnums populates the eval env with std module enums so
// FQN access like std.SvcState.restarted resolves at runtime. This
// mirrors check/std.go and will be replaced by stub-driven registration.
func (ev *Evaluator) registerStdEnums() {
	enums := map[string][]string{
		"PkgState":   {"present", "absent", "latest"},
		"SvcState":   {"running", "stopped", "restarted", "reloaded"},
		"UserState":  {"present", "absent"},
		"GroupState": {"present", "absent"},
		"CtrState":   {"running", "stopped", "absent"},
		"CtrRestart": {"always", "on_failure", "unless_stopped", "no"},
		"MountState": {"mounted", "unmounted", "absent"},
		"FsType":     {"nfs", "nfs4", "cifs", "ext4", "xfs", "btrfs", "tmpfs", "glusterfs", "ceph"},
		"FwAction":   {"allow", "deny", "reject"},
		"HttpMethod": {"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
	}
	stdMap := &MapVal{}
	for enumName, variants := range enums {
		variantMap := &MapVal{}
		for _, v := range variants {
			variantMap.Set(v, &StringVal{V: v})
		}
		stdMap.Set(enumName, variantMap)
	}
	ev.env.set("std", stdMap)
}

func (ev *Evaluator) errAt(span token.Span, msg string) {
	ev.errs = append(ev.errs, Error{Span: span, Msg: msg})
}

// evalFile evaluates a complete file.
func (ev *Evaluator) evalFile(f *ast.File) {
	// Evaluate declarations (registering functions, steps, structs).
	for _, d := range f.Decls {
		ev.evalDecl(d)
	}
	// Evaluate top-level statements (step invocations produce top-level values).
	for _, s := range f.Stmts {
		ev.evalStmt(s)
	}
}

// Declarations
// -----------------------------------------------------------------------------

func (ev *Evaluator) evalDecl(d ast.Decl) {
	switch d := d.(type) {
	case *ast.LetDecl:
		v := ev.evalExpr(d.Value)
		ev.env.set(d.Name.Name, v)
		ev.collectTopLevel(v)
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
	case *ast.StructDecl, *ast.EnumDecl:
		// Type declarations don't produce runtime values.
	}
}

// collectTopLevel checks if a value is a Target/Deploy/Secrets and
// adds it to the result.
func (ev *Evaluator) collectTopLevel(v Value) {
	if v == nil {
		return
	}
	switch val := v.(type) {
	case *TargetVal:
		ev.result.Targets = append(ev.result.Targets, val)
	case *DeployVal:
		ev.result.Deploys = append(ev.result.Deploys, val)
	case *SecretsVal:
		ev.result.Secrets = val
	}
}

// Statements
// -----------------------------------------------------------------------------

func (ev *Evaluator) evalStmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.ExprStmt:
		v := ev.evalExpr(s.Expr)
		if ev.currentDeploy != nil {
			if si, ok := v.(*StepVal); ok {
				ev.currentDeploy.Steps = append(ev.currentDeploy.Steps, si)
			}
		} else {
			ev.collectTopLevel(v)
		}
	case *ast.LetStmt:
		v := ev.evalExpr(s.Decl.Value)
		ev.env.set(s.Decl.Name.Name, v)
	case *ast.ForStmt:
		ev.evalFor(s)
	case *ast.IfStmt:
		ev.evalIf(s)
	case *ast.ReturnStmt:
		// Handled by callFunc.
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
	}
}

func (ev *Evaluator) evalFor(f *ast.ForStmt) {
	iter := ev.evalExpr(f.Iter)
	list, ok := iter.(*ListVal)
	if !ok {
		ev.errAt(f.SrcSpan, "for-in requires a list")
		return
	}
	for _, item := range list.Items {
		child := newEnv(ev.env)
		child.set(f.Var.Name, item)
		prev := ev.env
		ev.env = child
		ev.evalBlock(f.Body)
		ev.env = prev
	}
}

func (ev *Evaluator) evalIf(s *ast.IfStmt) {
	cond := ev.evalExpr(s.Cond)
	if asBool(cond) {
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
			ev.errAt(e.SrcSpan, "undefined: "+e.Name)
			return &NoneVal{}
		}
		return v
	case *ast.SelectorExpr:
		return ev.evalSelector(e)
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
		if asBool(cond) {
			return ev.evalExpr(e.Then)
		}
		return ev.evalExpr(e.Else)
	case *ast.ListComp:
		return ev.evalListComp(e)
	case *ast.DottedName:
		return ev.evalDottedName(e)
	}
	ev.errAt(e.Span(), "cannot evaluate expression")
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
	x := ev.evalExpr(sel.X)
	name := sel.Sel.Name
	if sv, ok := x.(*StructVal); ok {
		if v, exists := sv.Fields[name]; exists {
			return v
		}
	}
	if mv, ok := x.(*MapVal); ok {
		if v, exists := mv.Get(name); exists {
			return v
		}
	}
	ev.errAt(sel.SrcSpan, "cannot access ."+name)
	return &NoneVal{}
}

func (ev *Evaluator) evalDottedName(dn *ast.DottedName) Value {
	if len(dn.Parts) == 0 {
		return &NoneVal{}
	}
	v, ok := ev.env.get(dn.Parts[0].Name)
	if !ok {
		ev.errAt(dn.Parts[0].SrcSpan, "undefined: "+dn.Parts[0].Name)
		return &NoneVal{}
	}
	for _, part := range dn.Parts[1:] {
		if sv, ok := v.(*StructVal); ok {
			v = sv.Fields[part.Name]
			continue
		}
		if mv, ok := v.(*MapVal); ok {
			got, _ := mv.Get(part.Name)
			v = got
			continue
		}
		ev.errAt(part.SrcSpan, "cannot access ."+part.Name)
		return &NoneVal{}
	}
	return v
}

func (ev *Evaluator) evalCall(call *ast.CallExpr) Value {
	fn := ev.evalExpr(call.Fn)
	fv, ok := fn.(*FuncVal)
	if !ok {
		ev.errAt(call.SrcSpan, "cannot call non-function")
		return &NoneVal{}
	}
	argMap := make(map[string]Value, len(call.Args))
	var positional []Value
	for _, a := range call.Args {
		v := ev.evalExpr(a.Value)
		if a.Name != nil {
			argMap[a.Name.Name] = v
		} else {
			positional = append(positional, v)
		}
	}
	return ev.callFunc(fv, positional, argMap)
}

func (ev *Evaluator) callFunc(fv *FuncVal, positional []Value, kwargs map[string]Value) Value {
	body, ok := fv.body.(*ast.Block)
	if !ok {
		return &NoneVal{}
	}
	parent, _ := fv.scope.(*envScope)
	child := newEnv(parent)
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
	prev := ev.env
	ev.env = child
	var retVal Value
	for _, s := range body.Stmts {
		if rs, ok := s.(*ast.ReturnStmt); ok {
			if rs.Value != nil {
				retVal = ev.evalExpr(rs.Value)
			}
			break
		}
		ev.evalStmt(s)
	}
	ev.env = prev
	if retVal == nil {
		return &NoneVal{}
	}
	return retVal
}

func (ev *Evaluator) evalStructLit(lit *ast.StructLit) Value {
	fields := make(map[string]Value, len(lit.Fields))
	for _, f := range lit.Fields {
		fields[f.Name.Name] = ev.evalExpr(f.Value)
	}

	// Determine what kind of value this produces based on the type name.
	typeName := structLitTypeName(lit)

	// Step invocations (std.pkg, std.copy, etc.) → StepVal
	// Target invocations (target.ssh, etc.) → TargetVal.
	// Deploy invocations → DeployVal.
	// Secrets → SecretsVal.
	switch typeName {
	case "deploy":
		return ev.evalDeploy(fields, lit)
	case "secrets":
		return ev.evalSecrets(fields)
	case "ssh", "local", "rest":
		return ev.evalTarget(typeName, fields)
	default:
		if isStdStep(typeName) {
			return &StepVal{StepName: typeName, Fields: fields}
		}
		// User-defined step: look up in env, expand body with self bound.
		if fv, ok := ev.lookupStep(typeName); ok {
			return ev.expandUserStep(fv, fields)
		}
		return &StructVal{TypeName: typeName, Fields: fields}
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
	ev.evalBlock(stepDecl.Body)
	ev.env = prevEnv
	// The body's step invocations were already collected by the deploy
	// (if we're inside one) via the currentDeploy mechanism.
	return &NoneVal{}
}

func (ev *Evaluator) evalDeploy(
	fields map[string]Value,
	lit *ast.StructLit,
) *DeployVal {
	dv := &DeployVal{}
	if n, ok := fields["name"]; ok {
		if s, ok := n.(*StringVal); ok {
			dv.Name = s.V
		}
	}
	if t, ok := fields["targets"]; ok {
		if l, ok := t.(*ListVal); ok {
			dv.Targets = l.Items
		}
	}
	// Evaluate body statements. Bare step invocations inside the deploy
	// body are collected as desired-state steps.
	prev := ev.currentDeploy
	ev.currentDeploy = dv
	childEnv := newEnv(ev.env)
	prevEnv := ev.env
	ev.env = childEnv
	for _, s := range lit.Body {
		ev.evalStmt(s)
	}
	ev.env = prevEnv
	ev.currentDeploy = prev
	return dv
}

func (ev *Evaluator) evalSecrets(fields map[string]Value) *SecretsVal {
	sv := &SecretsVal{}
	if b, ok := fields["backend"]; ok {
		if s, ok := b.(*StringVal); ok {
			sv.Backend = s.V
		}
	}
	if p, ok := fields["path"]; ok {
		if s, ok := p.(*StringVal); ok {
			sv.Path = s.V
		}
	}
	return sv
}

func (ev *Evaluator) evalTarget(kind string, fields map[string]Value) *TargetVal {
	tv := &TargetVal{Kind: kind, Fields: fields}
	if n, ok := fields["name"]; ok {
		if s, ok := n.(*StringVal); ok {
			tv.Name = s.V
		}
	}
	return tv
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
		return intBinOp(lv, rv, func(a, b int64) int64 {
			if b == 0 {
				return 0
			}
			return a / b
		})
	case token.Percent:
		return intBinOp(lv, rv, func(a, b int64) int64 {
			if b == 0 {
				return 0
			}
			return a % b
		})
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
		return &BoolVal{V: lv.(*BoolVal).V && rv.(*BoolVal).V}
	case token.Or:
		return &BoolVal{V: lv.(*BoolVal).V || rv.(*BoolVal).V}
	case token.In:
		return &BoolVal{V: valueIn(lv, rv)}
	}
	return &NoneVal{}
}

func (ev *Evaluator) evalUnary(un *ast.UnaryExpr) Value {
	xv := ev.evalExpr(un.X)
	switch un.Op {
	case token.Not:
		return &BoolVal{V: !xv.(*BoolVal).V}
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
			if !asBool(cond) {
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

// asBool unwraps a BoolVal. The type checker guarantees only bool
// values reach here; a non-bool panics (compiler bug).
func asBool(v Value) bool {
	return v.(*BoolVal).V
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

func isStdStep(name string) bool {
	switch name {
	case "copy", "dir", "symlink", "template", "unarchive",
		"pkg", "service", "user", "group", "sysctl", "mount",
		"firewall", "run", "request", "resource",
		"local", "inline", "remote", "system",
		"apt_repo", "dnf_repo":
		return true
	}
	return false
}
