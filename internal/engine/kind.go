// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"fmt"
)

type Kind interface {
	Validate(r Resource) error
	// Identify names the attrs the engine must preserve after the
	// declared Resource is gone so Destroy can still find the live
	// thing. The engine sorts; return any order.
	Identify() Identity
	// Apply returns inSync=true when the resource was already in the
	// desired state and no work was done.
	Apply(ctx context.Context, r Resource, log Log) (inSync bool, err error)
	Destroy(ctx context.Context, ref Ref, attrs map[string]string, log Log) error
}

// Identity is a Kind's identifying attr names - the keys whose values
// survive into the inventory so Destroy can address the live resource
// once the desired Resource is gone.
type Identity []string

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
