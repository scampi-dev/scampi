// SPDX-License-Identifier: GPL-3.0-only

// Package eval is the scampi tree-walking evaluator. It takes a
// parsed and type-checked AST and produces generic runtime values.
// The evaluator has no knowledge of engine concepts — it just evaluates
// typed configuration language into values the caller interprets.
package eval

import "fmt"

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
type StructVal struct {
	TypeName string // leaf decl name ("copy", "ssh", "secrets")
	QualName string // qualified name ("posix.copy", "posix.ssh")
	RetType  string // return type from stubs ("Step", "Target", etc.)
	Fields   map[string]Value
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
	Params   []string
	Defaults []any  // parallel to Params: *ast.Expr or nil per param
	RetType  string // return type name for stubs (e.g. "block[Deploy]")
	body     any    // *ast.Block (nil for stubs)
	scope    any    // *envScope
}

func (*FuncVal) valueTag()        {}
func (v *FuncVal) String() string { return "func " + v.Name }

// Result
// -----------------------------------------------------------------------------

// Result is the output of evaluating a scampi program. It
// contains only generic typed values — no engine-specific types.
// The caller (linker) interprets these based on RetType/TypeName.
type Result struct {
	Bindings map[string]Value // all top-level let bindings
	Exprs    []Value          // top-level bare expressions (block fills, decl invocations)
}
