// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"context"

	"scampi.dev/scampi/internal/diagnostic/event"
)

// Ctx bundles a context.Context with the output Emitter so call sites
// thread one value, not a (ctx, em) pair. A Ctx is itself an Emitter.
type Ctx struct {
	context.Context
	em Emitter
}

func NewCtx(ctx context.Context, em Emitter) Ctx {
	return Ctx{Context: ctx, em: em}
}

// With rebinds to a derived base context — for fork points (errgroup,
// context.WithCancel).
func (c Ctx) With(ctx context.Context) Ctx {
	c.Context = ctx
	return c
}

func (c Ctx) Sink() Emitter { return c.em }

func (c Ctx) Emit(e event.Event) { c.em.Emit(e) }

func (c Ctx) Raise(err Raisable) { c.em.Raise(err) }
