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
			c.errAt(useSpan, "unknown attribute: @"+bare)
			return nil
		}
		at, _ := sym.Type.(*AttrType)
		return at
	case 2:
		modName := name.Parts[0].Name
		attrName := name.Parts[1].Name
		modSym := c.scope.Lookup(modName)
		if modSym == nil || modSym.Kind != SymImport {
			c.errAt(useSpan, "unknown module: "+modName)
			return nil
		}
		mod, ok := c.modules[modName]
		if !ok {
			c.errAt(useSpan, "unknown module: "+modName)
			return nil
		}
		sym := mod.Lookup("@" + attrName)
		if sym == nil || sym.Kind != SymAttrType {
			c.errAt(useSpan, "unknown attribute: @"+modName+"."+attrName)
			return nil
		}
		at, _ := sym.Type.(*AttrType)
		return at
	}
	c.errAt(useSpan, "attribute names may have at most one dot")
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
			c.errAt(a.SrcSpan, "marker attribute @"+at.Name+" takes no arguments")
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
							a.Positionals[0].Span(),
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
						p.Span(),
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
				a.Positionals[0].Span(),
				"cannot bind "+t.String()+" to attribute @"+at.Name+
					" field "+field.Name+" of type "+field.Type.String(),
			)
		}
		bound[field.Name] = true

	default:
		// Multiple positionals on a multi-field type (or a
		// single-field non-list type) — disallowed.
		c.errAt(
			a.SrcSpan,
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
			c.errAt(
				na.SrcSpan,
				"attribute @"+at.Name+" has no field "+na.Name.Name,
			)
			continue
		}
		if bound[na.Name.Name] {
			c.errAt(
				na.SrcSpan,
				"attribute @"+at.Name+" field "+na.Name.Name+
					" already bound by positional argument",
			)
			continue
		}
		t := c.typeOf(na.Value)
		if t != nil && !IsAssignableTo(t, field.Type) {
			c.errAt(
				na.Value.Span(),
				"cannot bind "+t.String()+" to attribute @"+at.Name+
					" field "+field.Name+" of type "+field.Type.String(),
			)
		}
		bound[na.Name.Name] = true
	}

	// Required fields (no default) must be bound.
	for _, f := range at.Fields {
		if !bound[f.Name] && !f.HasDef {
			c.errAt(
				a.SrcSpan,
				"attribute @"+at.Name+" missing required field "+f.Name,
			)
		}
	}
}

// isListType reports whether t is a List type. Used by bindAttribute
// to detect the variadic-sugar case for single-field attribute types.
func isListType(t Type) bool {
	_, ok := t.(*List)
	return ok
}
