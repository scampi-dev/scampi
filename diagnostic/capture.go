// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import "scampi.dev/scampi/diagnostic/event"

// Capture is an Emitter that buffers every event in arrival order.
// Use it where the pipeline expects an Emitter but the caller wants to
// inspect what was raised (LSP, tests, batch validators).
type Capture struct {
	Events []event.Event
}

func (c *Capture) Emit(e event.Event) { c.Events = append(c.Events, e) }
func (c *Capture) Raise(r Raisable)   { c.Emit(r.Diagnostic()) }
