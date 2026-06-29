// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"context"

	"scampi.dev/scampi/internal/diagnostic/event"
)

// Ctx bundles a context.Context with the Emitter so call sites thread one
// value, not a (ctx, em) pair. It is the only emit surface threaded through the
// codebase — functions that need to emit take a Ctx and call Emit/Raise; the
// bare *Emitter lives only at the NewEmitter → NewCtx boundary.
type Ctx struct {
	context.Context
	em *Emitter
}

func NewCtx(ctx context.Context, em *Emitter) Ctx {
	return Ctx{Context: ctx, em: em}
}

// With rebinds to a derived base context — for fork points (errgroup,
// context.WithCancel).
func (c Ctx) With(ctx context.Context) Ctx {
	c.Context = ctx
	return c
}

func (c Ctx) Emit(e event.Event) { c.em.Emit(e) }

func (c Ctx) Raise(err Raisable) { c.em.Raise(err) }

// Output returns the wrapped Output backend. For tests that supply a capturing
// Output and read back what was emitted; production code never needs it.
func (c Ctx) Output() Output { return c.em.out }
