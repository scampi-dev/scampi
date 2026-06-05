// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"fmt"
)

type Kind interface {
	Schema() KindSchema
	Validate(r Resource) error
	// Identify names the attrs that survive into the inventory so
	// Destroy can still address the live resource.
	Identify() Identity
	Check(ctx context.Context, r Resource) (State, error)
	Apply(ctx context.Context, r Resource, log Log) error
	Destroy(ctx context.Context, ref Ref, attrs Attrs, log Log) error
}

type KindSchema []AttrSpec

type AttrSpec struct {
	Name     string
	Type     ValueKind
	Required bool
	Default  Value // only inspected when Required == false
}

func Required(name string, t ValueKind) AttrSpec {
	return AttrSpec{Name: name, Type: t, Required: true}
}

func Optional(name string, t ValueKind, def Value) AttrSpec {
	return AttrSpec{Name: name, Type: t, Default: def}
}

func (s KindSchema) Find(name string) *AttrSpec {
	for i := range s {
		if s[i].Name == name {
			return &s[i]
		}
	}
	return nil
}

// commonAttrs apply to every kind. adopt steers whether the engine
// claims pre-existing live state instead of halting.
var commonAttrs = KindSchema{
	Optional("adopt", ValueBool, BoolValue(false)),
}

func effectiveSchema(k Kind) KindSchema {
	own := k.Schema()
	out := make(KindSchema, 0, len(own)+len(commonAttrs))
	out = append(out, own...)
	out = append(out, commonAttrs...)
	return out
}

type ValueKind int

const (
	ValueString ValueKind = iota
	ValueBool
)

func (k ValueKind) String() string {
	switch k {
	case ValueString:
		return "string"
	case ValueBool:
		return "bool"
	}
	return fmt.Sprintf("valueKind(%d)", int(k))
}

// Value is the typed contents of one resource attr. Kind discriminates
// which payload field is set; the other is zero.
type Value struct {
	Kind ValueKind
	Str  string
	Bool bool
}

func StringValue(s string) Value { return Value{Kind: ValueString, Str: s} }
func BoolValue(b bool) Value     { return Value{Kind: ValueBool, Bool: b} }

type Identity []string

type State int

const (
	StateMissing State = iota
	StateMatching
	StateDiverging
)

func (s State) String() string {
	switch s {
	case StateMissing:
		return "missing"
	case StateMatching:
		return "matching"
	case StateDiverging:
		return "diverging"
	}
	return fmt.Sprintf("state(%d)", int(s))
}

var kinds = map[string]Kind{
	"file": fileKind{},
	"dir":  dirKind{},
}

func kindFor(r Resource) (Kind, error) {
	k, ok := kinds[r.Kind]
	if !ok {
		return nil, fmt.Errorf("%s%s: unknown kind %q%s",
			r.Source.prefix(), r.Ref(), r.Kind, hintSuffix(r.Kind, kindNames()))
	}
	return k, nil
}

func kindNames() []string {
	out := make([]string, 0, len(kinds))
	for name := range kinds {
		out = append(out, name)
	}
	return out
}
