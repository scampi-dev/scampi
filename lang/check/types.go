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

// StructType is a user-defined struct (resolved from a TypeDecl with fields).
type StructType struct {
	Name   string
	Fields []*FieldDef
}

func (*StructType) typeTag()         {}
func (s *StructType) String() string { return s.Name }

// OpaqueType is a forward-declared type with no visible fields.
// Declared as `type Foo` (no braces). Values of this type can be
// passed around but never constructed or field-accessed in user code.
type OpaqueType struct {
	Name string
}

func (*OpaqueType) typeTag()         {}
func (o *OpaqueType) String() string { return o.Name }

// BlockType is `block[T]` — a value that needs a statement block to
// produce a T. Each fill produces an independent T value.
type BlockType struct {
	Inner Type // the type produced when the block is filled
}

func (*BlockType) typeTag() {}
func (b *BlockType) String() string {
	return "block[" + b.Inner.String() + "]"
}

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

// DeclType is the type of a decl declaration — distinct from FuncType
// because decl invocations use block syntax, not call syntax.
type DeclType struct {
	Name    string // may be dotted: "container.instance"
	Params  []*FieldDef
	Ret     Type // output type (StepInstance if not declared)
	HasBody bool
}

func (*DeclType) typeTag() {}
func (s *DeclType) String() string {
	return "decl " + s.Name + "(...) " + s.Ret.String()
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
	// Structural equality for collections.
	if sl, ok := src.(*List); ok {
		if dl, ok := dst.(*List); ok {
			return IsAssignableTo(sl.Elem, dl.Elem)
		}
	}
	if sm, ok := src.(*Map); ok {
		if dm, ok := dst.(*Map); ok {
			return IsAssignableTo(sm.Key, dm.Key) && IsAssignableTo(sm.Value, dm.Value)
		}
	}
	// Same named type.
	if ss, ok := src.(*StructType); ok {
		if ds, ok := dst.(*StructType); ok {
			return ss.Name == ds.Name
		}
	}
	if se, ok := src.(*EnumType); ok {
		if de, ok := dst.(*EnumType); ok {
			return se.Name == de.Name
		}
	}
	if so, ok := src.(*OpaqueType); ok {
		if do, ok := dst.(*OpaqueType); ok {
			return so.Name == do.Name
		}
	}
	return false
}
