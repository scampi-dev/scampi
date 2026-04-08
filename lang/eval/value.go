// SPDX-License-Identifier: GPL-3.0-only

// Package eval is the scampi-lang tree-walking evaluator. It takes a
// parsed and type-checked AST and produces runtime values: the typed
// outputs the engine consumes (targets, deploys, step instances).
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

// Struct instance
// -----------------------------------------------------------------------------

// StructVal is a runtime struct instance (field name → value).
type StructVal struct {
	TypeName string
	Fields   map[string]Value
}

func (*StructVal) valueTag() {}
func (v *StructVal) String() string {
	return v.TypeName + "{...}"
}

// Engine-level values
// -----------------------------------------------------------------------------

// StepVal represents a desired-state step invocation. It carries
// the step name and resolved field values for the engine.
type StepVal struct {
	StepName string
	Fields   map[string]Value
}

func (*StepVal) valueTag() {}
func (v *StepVal) String() string {
	return "Step(" + v.StepName + ")"
}

// TargetVal represents a resolved target declaration.
type TargetVal struct {
	Kind   string // "ssh", "local", "rest"
	Name   string
	Fields map[string]Value
}

func (*TargetVal) valueTag()        {}
func (v *TargetVal) String() string { return "Target(" + v.Name + ")" }

// BlockVal is a block handle — an unfilled block[T] value carrying
// config fields. Produced by func calls that return block[T]. Filled
// by a statement block to produce the inner value (e.g. DeployVal).
type BlockVal struct {
	Kind      string           // func name ("deploy", etc.)
	InnerType string           // the T in block[T] ("Deploy", etc.)
	Fields    map[string]Value // config fields from the call
}

func (*BlockVal) valueTag()        {}
func (v *BlockVal) String() string { return "block(" + v.Kind + ")" }

// DeployVal represents a resolved deploy declaration.
type DeployVal struct {
	Name    string
	Targets []Value
	Steps   []*StepVal
}

func (*DeployVal) valueTag()        {}
func (v *DeployVal) String() string { return "Deploy(" + v.Name + ")" }

// SecretsVal represents a resolved secrets configuration.
type SecretsVal struct {
	Backend string
	Path    string
}

func (*SecretsVal) valueTag()      {}
func (*SecretsVal) String() string { return "SecretsConfig" }

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

// Result is the output of evaluating a scampi-lang program.
type Result struct {
	Targets  []*TargetVal
	Deploys  []*DeployVal
	Secrets  *SecretsVal
	Bindings map[string]Value // all top-level let bindings
}
