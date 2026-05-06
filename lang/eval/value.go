// SPDX-License-Identifier: GPL-3.0-only

// Package eval is the scampi tree-walking evaluator. It takes a
// parsed and type-checked AST and produces generic runtime values.
// The evaluator has no knowledge of engine concepts — it just evaluates
// typed configuration language into values the caller interprets.
package eval

import (
	"fmt"

	"scampi.dev/scampi/lang/token"
)

// Value is a runtime value produced by evaluation. All values are
// immutable once created except for map/list contents inside func
// bodies (see the mutability rules in the spec).
type Value interface {
	valueTag()
	String() string
}

// Scalar values
// -----------------------------------------------------------------------------

// StringVal is a runtime string.
type StringVal struct{ V string }

func (*StringVal) valueTag()        {}
func (v *StringVal) String() string { return fmt.Sprintf("%q", v.V) }

// IntVal is a runtime integer.
type IntVal struct{ V int64 }

func (*IntVal) valueTag()        {}
func (v *IntVal) String() string { return fmt.Sprintf("%d", v.V) }

// BoolVal is a runtime boolean.
type BoolVal struct{ V bool }

func (*BoolVal) valueTag()        {}
func (v *BoolVal) String() string { return fmt.Sprintf("%t", v.V) }

// NoneVal is the absence marker for optional types.
type NoneVal struct{}

func (*NoneVal) valueTag()      {}
func (*NoneVal) String() string { return "none" }

// Collection values
// -----------------------------------------------------------------------------

// ListVal is a runtime list.
type ListVal struct{ Items []Value }

func (*ListVal) valueTag()        {}
func (v *ListVal) String() string { return fmt.Sprintf("[...%d items]", len(v.Items)) }

// MapVal is a runtime map. Preserves insertion order.
type MapVal struct {
	Keys   []Value
	Values []Value
}

func (*MapVal) valueTag() {}
func (v *MapVal) String() string {
	return fmt.Sprintf("{...%d entries}", len(v.Keys))
}

// Get looks up a key in the map. Returns (value, true) if found.
func (v *MapVal) Get(key string) (Value, bool) {
	for i, k := range v.Keys {
		if s, ok := k.(*StringVal); ok && s.V == key {
			return v.Values[i], true
		}
	}
	return nil, false
}

// Set inserts or updates a key. Used during func-body mutation.
func (v *MapVal) Set(key string, val Value) {
	for i, k := range v.Keys {
		if s, ok := k.(*StringVal); ok && s.V == key {
			v.Values[i] = val
			return
		}
	}
	v.Keys = append(v.Keys, &StringVal{V: key})
	v.Values = append(v.Values, val)
}

// Typed values
// -----------------------------------------------------------------------------

// StructVal is a runtime value produced by a decl invocation or a type
// literal. It carries the declaration's return type so the linker can
// interpret it without reparsing stubs.
//
// SrcSpan and FieldSpans carry the call-site source ranges so the
// linker can anchor diagnostics at the user's source. SrcSpan is the
// whole `Type { ... }` literal; FieldSpans[name] is the field's value
// expression. Both are zero when the StructVal is synthesised
// (e.g. wrapper-decl fallback) rather than produced from a literal.
type StructVal struct {
	TypeName   string // leaf decl name ("copy", "ssh", "secrets")
	QualName   string // qualified name ("posix.copy", "ssh.target")
	RetType    string // return type from stubs ("Step", "Target", etc.)
	Fields     map[string]Value
	SrcSpan    token.Span
	FieldSpans map[string]token.Span
}

func (*StructVal) valueTag() {}
func (v *StructVal) String() string {
	if v.RetType != "" {
		return v.RetType + "(" + v.TypeName + ")"
	}
	return v.TypeName + "{...}"
}

// BlockVal is an unfilled block[T] handle. Produced by func calls
// that return block[T]. Filled by a statement block to produce a
// BlockResultVal.
type BlockVal struct {
	FuncName  string           // func that created this ("deploy", etc.)
	InnerType string           // the T in block[T] ("Deploy", etc.)
	Fields    map[string]Value // config fields from the call
}

func (*BlockVal) valueTag()        {}
func (v *BlockVal) String() string { return "block(" + v.FuncName + ")" }

// BlockResultVal is a filled block[T] — the result of supplying a
// statement body to a block[T] value. It carries the config fields
// from the original call plus the values collected from the body.
type BlockResultVal struct {
	TypeName string           // the T in block[T] ("Deploy", etc.)
	FuncName string           // func that created the block ("deploy")
	Fields   map[string]Value // config from the call (name, targets, etc.)
	Body     []Value          // values collected from the body
}

func (*BlockResultVal) valueTag() {}
func (v *BlockResultVal) String() string {
	return v.TypeName + "(" + v.FuncName + ", " + fmt.Sprintf("%d body values", len(v.Body)) + ")"
}

// FuncVal is a callable function (carries its AST + closure scope).
// For stub funcs (no body), RetType indicates the return type name
// so the eval can produce the appropriate value.
type FuncVal struct {
	Name     string
	QualName string // module-qualified name (e.g. "test.target_in_memory")
	Params   []string
	Defaults []any  // parallel to Params: *ast.Expr or nil per param
	RetType  string // return type name for stubs (e.g. "block[Deploy]")
	body     any    // *ast.Block (nil for stubs)
	scope    any    // *envScope
}

func (*FuncVal) valueTag()        {}
func (v *FuncVal) String() string { return "func " + v.Name }

// ThunkVal is a deferred computation. Used for `pub let` exports of
// user modules (#269) so importing a module doesn't trigger eager
// evaluation of every exported binding (and every secret fetch /
// network call those bindings might do). The thunk is resolved on
// first access — at the selector / dotted-name lookup site that
// pulls the value out of the module's exported map.
type ThunkVal struct {
	eval   func() Value
	cached Value
	forced bool
}

func (*ThunkVal) valueTag() {}
func (v *ThunkVal) String() string {
	if v.forced {
		return v.cached.String()
	}
	return "<thunk>"
}

// Force evaluates the thunk if it hasn't been already, caches the
// result, and returns it. Subsequent calls are O(1).
func (v *ThunkVal) Force() Value {
	if !v.forced {
		v.cached = v.eval()
		v.forced = true
	}
	return v.cached
}

// forceValue collapses a ThunkVal to its computed value; passes other
// values through unchanged. Call at every site where a value is
// pulled out of a container that may hold thunks (module pubMap,
// fullMap, etc.) — keeps thunk semantics from leaking past the
// access boundary.
func forceValue(v Value) Value {
	if t, ok := v.(*ThunkVal); ok {
		return t.Force()
	}
	return v
}

// RefVal is a cross-step value reference. Produced by std.ref(step, expr)
// at eval time. The linker converts it to a spec.Ref with a concrete
// StepID once step linking assigns IDs.
type RefVal struct {
	Step *StructVal // the step being referenced (eval-time identity)
	Expr string     // jq expression to extract from the step's output
}

func (*RefVal) valueTag()        {}
func (v *RefVal) String() string { return "ref(..," + v.Expr + ")" }

// OpaqueVal is a runtime value opaque to the evaluator. It wraps
// an arbitrary Go object produced by a caller-registered BuiltinFunc
// during eval. Unlike StructVal (which holds user-provided config
// fields as map[string]Value), OpaqueVal carries runtime state that
// the eval layer cannot interpret — the caller constructs it and
// later type-asserts Inner to recover the concrete type.
//
// Example: secrets.from_age() constructs a secret.Backend at eval
// time. The eval layer stores it as OpaqueVal{Inner: backend}; the
// linker's attribute static-check pass type-asserts Inner back to
// secret.Backend to validate literal keys.
type OpaqueVal struct {
	TypeName string // matches the stub's return type (e.g. "SecretResolver")
	Inner    any    // concrete Go value — eval never touches this
}

func (*OpaqueVal) valueTag()        {}
func (v *OpaqueVal) String() string { return v.TypeName }

// Result
// -----------------------------------------------------------------------------

// Result is the output of evaluating a scampi program. It
// contains only generic typed values — no engine-specific types.
// The caller (linker) interprets these based on RetType/TypeName.
type Result struct {
	Bindings map[string]Value // all top-level let bindings
	Exprs    []Value          // top-level bare expressions (block fills, decl invocations)
}
