// SPDX-License-Identifier: GPL-3.0-only

package check

import (
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
)

// checkFieldAttributes resolves and type-checks every attribute
// attached to a field. Each attribute reference is looked up in the
// `@`-prefixed namespace, then its arguments are bound against the
// declared schema using the rules described in BindAttribute.
func (c *Checker) checkFieldAttributes(f *ast.Field) {
	for _, a := range f.Attributes {
		c.checkAttribute(a)
	}
}

// resolveFieldAttributes returns the resolved attribute annotations
// on a field, in declaration order. Each entry carries the fully
// qualified attribute type name, the bound argument values resolved
// from literal expressions, and a pointer to the AttrType schema
// itself. Downstream consumers (linker behaviours, LSP hover) can
// read .Type.Doc and .Type.Fields directly without re-walking the
// module scope. Non-literal expressions resolve to nil in Args;
// behaviours that need a literal value should defer to runtime
// checks when they encounter nil.
func (c *Checker) resolveFieldAttributes(f *ast.Field) []ResolvedAttribute {
	if len(f.Attributes) == 0 {
		return nil
	}
	out := make([]ResolvedAttribute, 0, len(f.Attributes))
	for _, a := range f.Attributes {
		qn := c.qualifiedAttrName(a)
		if qn == "" {
			continue
		}
		ra := ResolvedAttribute{
			QualifiedName: qn,
			Args:          c.resolveAttributeArgs(a),
			Type:          c.lookupAttrTypeForArgs(a.Name),
		}
		out = append(out, ra)
	}
	return out
}

// qualifiedAttrName builds the fully qualified attribute type name
// for an attribute reference. Single-segment names use the current
// module's name; two-segment names pass through.
func (c *Checker) qualifiedAttrName(a *ast.Attribute) string {
	parts := a.Name.Parts
	switch len(parts) {
	case 1:
		return c.modName + ".@" + parts[0].Name
	case 2:
		return parts[0].Name + ".@" + parts[1].Name
	}
	return ""
}

// resolveAttributeArgs converts an attribute reference's argument
// expressions into a map of resolved Go values. Positional arguments
// bind to the attribute type's first field (or are wrapped into a
// list for variadic single-list-field types); named arguments bind
// by name. Non-literal arguments resolve to nil — behaviours should
// check for nil before using a value.
//
// Resolution is best-effort: the type checker has already validated
// the binding shape, so we don't re-emit errors here.
func (c *Checker) resolveAttributeArgs(a *ast.Attribute) map[string]any {
	at := c.lookupAttrTypeForArgs(a.Name)
	if at == nil {
		return nil
	}
	out := make(map[string]any)

	// Variadic-positional case: single list field, multiple positionals.
	if len(at.Fields) == 1 && isListType(at.Fields[0].Type) && len(a.Positionals) > 0 && len(a.Named) == 0 {
		field := at.Fields[0]
		// One positional that's already a list literal binds directly.
		if len(a.Positionals) == 1 {
			if v, ok := literalValue(a.Positionals[0]); ok {
				out[field.Name] = v
				return out
			}
		}
		items := make([]any, 0, len(a.Positionals))
		for _, p := range a.Positionals {
			if v, ok := literalValue(p); ok {
				items = append(items, v)
			}
		}
		out[field.Name] = items
		return out
	}

	// Single positional → first field.
	if len(a.Positionals) == 1 && len(at.Fields) > 0 {
		field := at.Fields[0]
		if v, ok := literalValue(a.Positionals[0]); ok {
			out[field.Name] = v
		}
	}
	for _, na := range a.Named {
		if v, ok := literalValue(na.Value); ok {
			out[na.Name.Name] = v
		}
	}

	return out
}

// lookupAttrTypeForArgs is a side-effect-free lookup of an attribute
// type by its dotted name reference. Used by resolveAttributeArgs to
// find the schema for binding without emitting diagnostics — those
// are handled separately by checkAttribute.
func (c *Checker) lookupAttrTypeForArgs(name *ast.DottedName) *AttrType {
	switch len(name.Parts) {
	case 1:
		sym := c.scope.Lookup("@" + name.Parts[0].Name)
		if sym == nil || sym.Kind != SymAttrType {
			return nil
		}
		at, _ := sym.Type.(*AttrType)
		return at
	case 2:
		mod, ok := c.modules[name.Parts[0].Name]
		if !ok {
			return nil
		}
		sym := mod.Lookup("@" + name.Parts[1].Name)
		if sym == nil || sym.Kind != SymAttrType {
			return nil
		}
		at, _ := sym.Type.(*AttrType)
		return at
	}
	return nil
}

// literalValue extracts a Go value from a literal AST expression.
// Returns false for non-literal expressions (variables, calls,
// arithmetic, etc.) — those aren't supported in attribute arguments.
func literalValue(e ast.Expr) (any, bool) {
	switch v := e.(type) {
	case *ast.StringLit:
		if len(v.Parts) == 1 {
			if t, ok := v.Parts[0].(*ast.StringText); ok {
				return t.Raw, true
			}
		}
	case *ast.IntLit:
		return v.Value, true
	case *ast.BoolLit:
		return v.Value, true
	case *ast.NoneLit:
		return nil, true
	case *ast.ListLit:
		items := make([]any, 0, len(v.Items))
		for _, it := range v.Items {
			val, ok := literalValue(it)
			if !ok {
				return nil, false
			}
			items = append(items, val)
		}
		return items, true
	}
	return nil, false
}

// checkAttribute resolves a single attribute reference and binds its
// arguments. Errors are accumulated on the checker.
func (c *Checker) checkAttribute(a *ast.Attribute) {
	at := c.resolveAttrType(a.Name, a.SrcSpan)
	if at == nil {
		return
	}
	c.bindAttribute(a, at)
}

// resolveAttrType looks up an attribute type by its dotted name.
// Single-segment names resolve in the local file scope under
// `@<name>`. Two-segment names (`@module.name`) resolve to an
// imported module's `@<name>` symbol.
func (c *Checker) resolveAttrType(name *ast.DottedName, useSpan token.Span) *AttrType {
	switch len(name.Parts) {
	case 1:
		bare := name.Parts[0].Name
		sym := c.scope.Lookup("@" + bare)
		if sym == nil || sym.Kind != SymAttrType {
			c.errAt(useSpan, CodeUnknownAttribute, "unknown attribute: @"+bare)
			return nil
		}
		at, _ := sym.Type.(*AttrType)
		return at
	case 2:
		modName := name.Parts[0].Name
		attrName := name.Parts[1].Name
		modSym := c.scope.Lookup(modName)
		if modSym == nil || modSym.Kind != SymImport {
			c.errAt(useSpan, CodeUnknownModule, "unknown module: "+modName)
			return nil
		}
		mod, ok := c.modules[modName]
		if !ok {
			c.errAt(useSpan, CodeUnknownModule, "unknown module: "+modName)
			return nil
		}
		sym := mod.Lookup("@" + attrName)
		if sym == nil || sym.Kind != SymAttrType {
			c.errAt(useSpan, CodeUnknownAttribute, "unknown attribute: @"+modName+"."+attrName)
			return nil
		}
		at, _ := sym.Type.(*AttrType)
		return at
	}
	c.errAt(useSpan, CodeAttrTooManyDots, "attribute names may have at most one dot")
	return nil
}

// bindAttribute applies the argument-binding rules to a single
// attribute reference and validates each bound value against the
// corresponding declared field's type. The binding rules are:
//
//  1. Marker (zero fields): no arguments accepted
//  2. Single non-list field: at most one positional binds to the
//     field; otherwise must be named
//  3. Single list field: positionals are sugar — they're wrapped
//     into an implicit list literal and bound. A lone positional
//     that is itself a list literal binds directly. Variadic always
//     picks the wrap interpretation when there are multiple args.
//  4. Multi-field: at most one positional argument (binds to the
//     first declared field); all subsequent args must be named
//  5. Variadic positional is only legal for single-field list types
func (c *Checker) bindAttribute(a *ast.Attribute, at *AttrType) {
	// Marker: no args allowed.
	if len(at.Fields) == 0 {
		if len(a.Positionals) > 0 || len(a.Named) > 0 {
			c.errAt(a.SrcSpan, CodeMarkerAttrArgs, "marker attribute @"+at.Name+" takes no arguments")
		}
		return
	}

	// Build a map of field name → field def for keyword binding
	// and a "bound" set so we can detect missing required fields.
	byName := make(map[string]*FieldDef, len(at.Fields))
	for _, f := range at.Fields {
		byName[f.Name] = f
	}
	bound := make(map[string]bool, len(at.Fields))

	// Bind positionals to fields. The rules above determine the
	// shape — we apply them strictly here.
	switch {
	case len(a.Positionals) == 0:
		// Nothing to bind positionally.

	case len(at.Fields) == 1 && isListType(at.Fields[0].Type):
		// Single list field: variadic sugar. If exactly one
		// positional is itself a list literal, bind it directly;
		// otherwise wrap all positionals into an implicit list and
		// type-check each element against the list's element type.
		field := at.Fields[0]
		listT := field.Type.(*List)
		if len(a.Positionals) == 1 {
			if t := c.typeOf(a.Positionals[0]); t != nil {
				if !IsAssignableTo(t, field.Type) {
					// Not the list itself — try as a single element.
					if !IsAssignableTo(t, listT.Elem) {
						c.errAt(
							a.Positionals[0].Span(), "lang.AttrBindError",
							"cannot bind "+t.String()+" to attribute @"+at.Name+
								" field "+field.Name+" of type "+field.Type.String(),
						)
					}
				}
			}
		} else {
			for _, p := range a.Positionals {
				t := c.typeOf(p)
				if t == nil {
					continue
				}
				if !IsAssignableTo(t, listT.Elem) {
					c.errAt(
						p.Span(), CodeAttrError,
						"cannot bind "+t.String()+" to attribute @"+at.Name+
							" field "+field.Name+" element type "+listT.Elem.String(),
					)
				}
			}
		}
		bound[field.Name] = true

	case len(a.Positionals) == 1:
		// Single positional binds to first field (whether single- or
		// multi-field type).
		field := at.Fields[0]
		t := c.typeOf(a.Positionals[0])
		if t != nil && !IsAssignableTo(t, field.Type) {
			c.errAt(
				a.Positionals[0].Span(), CodeAttrError,
				"cannot bind "+t.String()+" to attribute @"+at.Name+
					" field "+field.Name+" of type "+field.Type.String(),
			)
		}
		bound[field.Name] = true

	default:
		// Multiple positionals on a multi-field type (or a
		// single-field non-list type) — disallowed.
		c.errAt(
			a.SrcSpan, CodeAttrError,
			"attribute @"+at.Name+" accepts at most one positional argument; "+
				"use keyword arguments for the rest",
		)
		return
	}

	// Bind named arguments. Each must reference a declared field and
	// not collide with the positional binding.
	for _, na := range a.Named {
		field, ok := byName[na.Name.Name]
		if !ok {
			c.errAt(na.SrcSpan, CodeAttrError, "attribute @"+at.Name+" has no field "+na.Name.Name)
			continue
		}
		if bound[na.Name.Name] {
			c.errAt(
				na.SrcSpan, CodeAttrError,
				"attribute @"+at.Name+" field "+na.Name.Name+
					" already bound by positional argument",
			)
			continue
		}
		t := c.typeOf(na.Value)
		if t != nil && !IsAssignableTo(t, field.Type) {
			c.errAt(
				na.Value.Span(), "lang.AttrBindError",
				"cannot bind "+t.String()+" to attribute @"+at.Name+
					" field "+field.Name+" of type "+field.Type.String(),
			)
		}
		bound[na.Name.Name] = true
	}

	// Required fields (no default) must be bound.
	for _, f := range at.Fields {
		if !bound[f.Name] && !f.HasDef {
			c.errAt(a.SrcSpan, CodeAttrError, "attribute @"+at.Name+" missing required field "+f.Name)
		}
	}
}

// isListType reports whether t is a List type. Used by bindAttribute
// to detect the variadic-sugar case for single-field attribute types.
func isListType(t Type) bool {
	_, ok := t.(*List)
	return ok
}
