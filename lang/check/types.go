// SPDX-License-Identifier: GPL-3.0-only

// Package check implements the scampi-lang type system and type checker.
// It operates on parsed ASTs and resolves types, scopes, and validity.
package check

import "slices"

// Type is the semantic type representation. All resolved types
// implement this interface. Types are comparable by identity (pointer
// equality) for builtin types, and structurally for user-defined types.
type Type interface {
	typeTag() // sealed
	String() string
}

// Builtin scalar types. These are singletons — compare with ==.
var (
	StringType = &Builtin{Name: "string"}
	IntType    = &Builtin{Name: "int"}
	BoolType   = &Builtin{Name: "bool"}
	NoneType   = &Builtin{Name: "none"} // only inhabits T?
	AnyType    = &Builtin{Name: "any"}  // escape hatch for map[string, any]
)

// Engine-level opaque types produced by step invocations.
var (
	StepInstanceType  = &Builtin{Name: "StepInstance"}
	TargetType        = &Builtin{Name: "Target"}
	DeployType        = &Builtin{Name: "Deploy"}
	SecretsConfigType = &Builtin{Name: "SecretsConfig"}
)

// Composable value types (plugged into step fields).
var (
	SourceType    = &Builtin{Name: "Source"}
	PkgSourceType = &Builtin{Name: "PkgSource"}
	AuthType      = &Builtin{Name: "Auth"}
	TLSType       = &Builtin{Name: "TLS"}
	BodyType      = &Builtin{Name: "Body"}
	CheckType     = &Builtin{Name: "Check"}
)

// Builtin is a named primitive or opaque type.
type Builtin struct{ Name string }

func (*Builtin) typeTag()         {}
func (b *Builtin) String() string { return b.Name }

// Optional wraps a type as T?.
type Optional struct{ Inner Type }

func (*Optional) typeTag()         {}
func (o *Optional) String() string { return o.Inner.String() + "?" }

// List is list[T].
type List struct{ Elem Type }

func (*List) typeTag()         {}
func (l *List) String() string { return "list[" + l.Elem.String() + "]" }

// Map is map[K, V].
type Map struct {
	Key   Type
	Value Type
}

func (*Map) typeTag()         {}
func (m *Map) String() string { return "map[" + m.Key.String() + ", " + m.Value.String() + "]" }

// StructType is a user-defined struct (resolved from a StructDecl).
type StructType struct {
	Name   string
	Fields []*FieldDef
}

func (*StructType) typeTag()         {}
func (s *StructType) String() string { return s.Name }

// FieldDef is a field in a struct or step declaration.
type FieldDef struct {
	Name   string
	Type   Type
	HasDef bool // true if the field has a default value
}

// EnumType is a user-defined enum.
type EnumType struct {
	Name     string
	Variants []string
}

func (*EnumType) typeTag()         {}
func (e *EnumType) String() string { return e.Name }

// HasVariant reports whether v is a valid variant of this enum.
func (e *EnumType) HasVariant(v string) bool {
	return slices.Contains(e.Variants, v)
}

// FuncType is the type of a function value.
type FuncType struct {
	Params []*FieldDef
	Ret    Type
}

func (*FuncType) typeTag() {}
func (f *FuncType) String() string {
	return "func(...) " + f.Ret.String()
}

// StepType is the type of a step declaration — distinct from FuncType
// because step invocations use block syntax, not call syntax.
type StepType struct {
	Name    string // may be dotted: "container.instance"
	Params  []*FieldDef
	Ret     Type // output type (StepInstance if not declared)
	HasBody bool
}

func (*StepType) typeTag() {}
func (s *StepType) String() string {
	return "step " + s.Name + "(...) " + s.Ret.String()
}

// IsAssignableTo reports whether a value of type src can be used
// where type dst is expected. Handles optional promotion (T → T?),
// none → T?, and any escape hatch.
func IsAssignableTo(src, dst Type) bool {
	if src == dst {
		return true
	}
	// any accepts everything.
	if dst == AnyType {
		return true
	}
	// T is assignable to T?.
	if opt, ok := dst.(*Optional); ok {
		if src == NoneType {
			return true
		}
		return IsAssignableTo(src, opt.Inner)
	}
	return false
}
