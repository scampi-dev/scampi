// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"strings"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/lang/token"
	"scampi.dev/scampi/spec"
)

// runAttributeEvalChecks walks the eval result for every StructVal
// produced by a decl invocation and dispatches each annotated param's
// behaviour against the eval-resolved value.
//
// This complements the AST-walk in attribute_check.go: the AST-walk
// covers func/UFCS calls (where eval has consumed the args), the
// eval-walk covers decl invocations (where eval preserves them in
// StructVal.Fields). Comprehensions and nested wrappers naturally
// produce one StructVal per iteration so every concrete instance is
// validated against the resolved value rather than the syntactic
// literal.
//
// Diagnostics are deduped downstream at the diagnostic.policyEmitter
// boundary, so overlapping emissions from both walkers (which is
// expected during the Step 3/4 transition) collapse to one user-
// visible message per content key.
func runAttributeEvalChecks(
	result *eval.Result,
	source []byte,
	cfgPath string,
	fileScope *check.Scope,
	modules map[string]*check.Scope,
	registry *AttributeRegistry,
) error {
	if registry == nil || result == nil {
		return nil
	}
	ctx := &linkContext{}
	eval.WalkResult(result, func(v eval.Value) bool {
		sv, ok := v.(*eval.StructVal)
		if !ok {
			return true
		}
		dt := lookupDeclTypeForStructVal(sv, fileScope, modules)
		if dt == nil {
			return true
		}
		dispatchEvalAttributes(ctx, sv, dt, registry, source, cfgPath)
		return true
	})
	if len(ctx.diags) == 0 {
		return nil
	}
	return ctx.diags
}

// lookupDeclTypeForStructVal resolves a StructVal's DeclType via its
// qualified or leaf name. Mirrors resolveStructLitDecl in the AST-walk
// path but starts from the eval-side string rather than an AST node.
// Returns nil for anonymous struct values, types declared as `type`
// (not `decl`), or names absent from the resolved scopes.
func lookupDeclTypeForStructVal(
	sv *eval.StructVal,
	fileScope *check.Scope,
	modules map[string]*check.Scope,
) *check.DeclType {
	if i := strings.IndexByte(sv.QualName, '.'); i >= 0 {
		modName := sv.QualName[:i]
		leafName := sv.QualName[i+1:]
		mod, ok := modules[modName]
		if !ok || mod == nil {
			return nil
		}
		sym := mod.Lookup(leafName)
		if sym == nil {
			return nil
		}
		dt, _ := sym.Type.(*check.DeclType)
		return dt
	}
	if fileScope == nil || sv.TypeName == "" {
		return nil
	}
	sym := fileScope.Lookup(sv.TypeName)
	if sym == nil {
		return nil
	}
	dt, _ := sym.Type.(*check.DeclType)
	return dt
}

// dispatchEvalAttributes runs every registered behaviour for every
// annotated parameter on the given StructVal.
func dispatchEvalAttributes(
	ctx LinkContext,
	sv *eval.StructVal,
	dt *check.DeclType,
	registry *AttributeRegistry,
	source []byte,
	cfgPath string,
) {
	if len(dt.Params) == 0 {
		return
	}
	for _, p := range dt.Params {
		if len(p.Attributes) == 0 {
			continue
		}
		evalVal, ok := sv.Fields[p.Name]
		if !ok {
			continue // optional, omitted at the call site
		}
		// NoneVal: literal `none` or absent optional — skip; behaviours
		// can't validate "no value provided".
		if _, isNone := evalVal.(*eval.NoneVal); isNone {
			continue
		}
		// RefVal: cross-step reference (std.ref). The actual value
		// only resolves at runtime; defer validation.
		if _, isRef := evalVal.(*eval.RefVal); isRef {
			continue
		}
		resolved := evalToGo(evalVal)
		useSpan := evalSpanToSourceSpan(sv.FieldSpans[p.Name], source, cfgPath)
		for _, attr := range p.Attributes {
			behaviour := registry.Lookup(attr.QualifiedName)
			if behaviour == nil {
				continue
			}
			behaviour.StaticCheck(StaticCheckContext{
				Linker:    ctx,
				AttrName:  attr.QualifiedName,
				AttrArgs:  attr.Args,
				AttrDoc:   attrDoc(attr),
				ParamName: p.Name,
				Resolved:  resolved,
				UseSpan:   useSpan,
			})
		}
	}
}

// evalSpanToSourceSpan converts an eval-side token span to a
// renderable spec.SourceSpan, anchored on the entry-point file.
// Returns a Filename-only span when source bytes aren't available
// (e.g. tests that don't plumb source through Analyze).
func evalSpanToSourceSpan(span token.Span, source []byte, cfgPath string) spec.SourceSpan {
	if span.End == 0 || len(source) == 0 {
		return spec.SourceSpan{Filename: cfgPath}
	}
	sLine, sCol := offsetToLineCol(source, int(span.Start))
	eLine, eCol := offsetToLineCol(source, int(span.End))
	return spec.SourceSpan{
		Filename:  cfgPath,
		StartLine: sLine,
		StartCol:  sCol,
		EndLine:   eLine,
		EndCol:    eCol,
	}
}
