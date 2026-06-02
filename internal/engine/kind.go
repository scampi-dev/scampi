// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"fmt"
)

// Kind is the per-Kind contract. Each implementation owns its own
// validation and apply behavior; the engine dispatches via the kinds
// registry and stays Kind-agnostic.
type Kind interface {
	Validate(r Resource) error
	Apply(ctx context.Context, r Resource, log Log) error
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
