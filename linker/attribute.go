// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/spec"
)

// ResolverBackendFunc extracts a secret.Backend from the eval result
// for a given receiver expression (typically a let-bound variable).
// Used by @secretkey to validate literal keys against the specific
// resolver used in a UFCS get() call.
type ResolverBackendFunc func(receiverName string) secret.Backend

// AttributeBehaviour is the scampi-specific semantics attached to an
// attribute type. Lang owns the schema (declared via `type @name { ... }`
// in stubs); the linker owns what an attribute *means* at link time.
// The LSP separately owns UX semantics (completion, hover) keyed by
// the same qualified name.
//
// Implementations are looked up in an [AttributeRegistry] keyed by the
// fully qualified attribute type name (e.g. `std.@secretkey`,
// `posix.@path`).
//
// Eval-time checks for dynamic argument values are not yet wired
// (#159 follow-up). For now the existing per-builtin runtime checks
// in lang/eval continue to handle dynamic args; StaticCheck handles
// the literal-args case.
type AttributeBehaviour interface {
	// StaticCheck is invoked at link time, once per call site of a
	// function whose parameter carries this attribute. The supplied
	// StaticCheckContext exposes the user's call-site argument
	// expression (Param), the attribute reference itself (Attribute,
	// for behaviours that read the attribute's own args like
	// `@pattern("...")`), the linker context for diagnostic emission
	// and backend lookup, and the source span of the use.
	//
	// Implementations should emit diagnostics through ctx.Linker
	// rather than returning them so the standard pipeline (source
	// spans, --color, --json) handles formatting.
	StaticCheck(ctx StaticCheckContext)
}

// StaticCheckContext carries everything an AttributeBehaviour needs
// to validate a single annotated call site. Each StaticCheck hook
// receives one of these per use of its attribute.
type StaticCheckContext struct {
	// Linker exposes diagnostic emission.
	Linker LinkContext

	// ResolverBackend is the secret.Backend extracted from the
	// resolver argument of the enclosing call, when available.
	// Used by @secretkey to validate literal keys against the
	// specific resolver rather than a global backend.
	ResolverBackend secret.Backend

	// AttrName is the fully qualified attribute type name (e.g.
	// `std.@pattern`). Behaviours can use this for diagnostic
	// messages without hardcoding their own name.
	AttrName string

	// AttrArgs holds the resolved literal arguments of the attribute
	// reference, bound by field name. Markers (`@nonempty`) get an
	// empty map; behaviours that take their own args (like
	// `@pattern(regex)`) read them via the field name. Non-literal
	// or absent args are missing from the map.
	AttrArgs map[string]any

	// AttrDoc is the doc-comment block declared above the attribute
	// type's `type @name { ... }` declaration in the source. The
	// linker uses it as the Help text on validation diagnostics so
	// the rich content lives in one place: the attribute type
	// declaration in std/std.scampi (or whichever module declares
	// it). Empty if no doc comment is present.
	AttrDoc string

	// ParamName is the declared name of the parameter that carries
	// this attribute (e.g. "name" for `@secretkey name: string`).
	ParamName string

	// ParamArg is the user's call-site argument expression bound to
	// the annotated parameter. May be a literal that the behaviour
	// can validate eagerly, or any other expression for which the
	// behaviour should defer to the runtime check.
	ParamArg ast.Expr

	// UseSpan is the source span of the call-site argument, for
	// anchoring diagnostics.
	UseSpan spec.SourceSpan
}

// LinkContext is the linker-side context passed to an attribute's
// StaticCheck hook. It exposes a diagnostic emitter without
// requiring the attribute to import the entire linker package.
type LinkContext interface {
	// Emit records a diagnostic with the standard pipeline.
	Emit(d diagnostic.Diagnostic)
}

// BoundArg is a single argument bound to a declared field of an
// attribute type, in its raw AST form. Behaviours inspect the
// expression to detect literal values they can validate eagerly.
//
// Deprecated: kept for transitional compatibility with existing
// callers; prefer reading directly from StaticCheckContext fields.
type BoundArg struct {
	Field   string   // declared field name
	Value   ast.Expr // raw expression (may be a literal or a variable)
	SrcSpan spec.SourceSpan
}

// AttributeRegistry holds the AttributeBehaviour for every attribute
// type the linker recognises, keyed by the fully qualified attribute
// name (with the leading `@`, e.g. `std.@secretkey`).
//
// User-defined attribute types declared in third-party scampi modules
// are intentionally absent from this registry — they type-check at
// the lang level but have no runtime behaviour. Future tooling (or a
// future lang-level hook mechanism, see #159 future work) can attach
// behaviour to them.
type AttributeRegistry struct {
	behaviours map[string]AttributeBehaviour
}

// NewAttributeRegistry returns an empty registry. Use Register to
// add behaviours, or DefaultAttributes for the standard set.
func NewAttributeRegistry() *AttributeRegistry {
	return &AttributeRegistry{
		behaviours: make(map[string]AttributeBehaviour),
	}
}

// Register adds a behaviour for the named attribute type. The name
// must be the fully qualified form including the leading `@` (e.g.
// `std.@secretkey`). Subsequent registrations with the same name
// overwrite the previous one.
func (r *AttributeRegistry) Register(qualifiedName string, b AttributeBehaviour) {
	r.behaviours[qualifiedName] = b
}

// Lookup returns the behaviour for the named attribute, or nil if
// none is registered. A nil result means the attribute is inert at
// the linker layer (lang still type-checks it, LSP still consumes it).
func (r *AttributeRegistry) Lookup(qualifiedName string) AttributeBehaviour {
	return r.behaviours[qualifiedName]
}

// Names returns the qualified names of every registered attribute,
// in unspecified order. Useful for diagnostics ("did you mean…?")
// and for tests.
func (r *AttributeRegistry) Names() []string {
	out := make([]string, 0, len(r.behaviours))
	for name := range r.behaviours {
		out = append(out, name)
	}
	return out
}

// DefaultAttributes returns a registry populated with the standard
// scampi attribute behaviours. Each entry validates literal arguments
// at link time and emits typed diagnostics through the standard
// pipeline. Behaviours that need their own arguments (`@pattern`,
// `@oneof`, `@deprecated`, `@path`) read them from the resolved
// AttrArgs map carried on each StaticCheckContext.
func DefaultAttributes() *AttributeRegistry {
	r := NewAttributeRegistry()
	r.Register("secrets.@secretkey", SecretKeyAttribute{})
	r.Register("std.@nonempty", NonEmptyAttribute{})
	r.Register("std.@filemode", FileModeAttribute{})
	r.Register("std.@size", SizeAttribute{})
	r.Register("std.@pattern", PatternAttribute{})
	r.Register("std.@oneof", OneOfAttribute{})
	r.Register("std.@deprecated", DeprecatedAttribute{})
	r.Register("std.@since", SinceAttribute{})
	r.Register("std.@path", PathAttribute{})
	r.Register("std.@min", MinAttribute{})
	r.Register("std.@max", MaxAttribute{})
	return r
}
