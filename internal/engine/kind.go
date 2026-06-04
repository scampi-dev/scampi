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

type KindSchema struct {
	Required []string
	Optional []string
}

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
		return nil, fmt.Errorf("%s: unknown kind", r.Ref())
	}
	return k, nil
}
