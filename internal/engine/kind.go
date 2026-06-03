// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"fmt"
)

type Kind interface {
	Validate(r Resource) error
	// Identify names the attrs the engine must preserve after the
	// declared Resource is gone so Destroy can still address the
	// live resource.
	Identify() Identity
	Check(ctx context.Context, r Resource) (State, error)
	Apply(ctx context.Context, r Resource, log Log) error
	Destroy(ctx context.Context, ref Ref, attrs Attrs, log Log) error
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

// kinds maps the HCL block name to its behavior. Adding a Kind means
// adding a file (kind_X.go) and a line here.
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
