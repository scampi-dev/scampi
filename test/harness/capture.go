// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
)

// Capture is an Emitter that buffers every event in arrival order. It
// satisfies diagnostic.Emitter for tests that want to inspect what a
// producer raised without going through the full RecordingDisplayer
// pipeline.
type Capture struct {
	Events []event.Event
}

func (c *Capture) Emit(e event.Event)          { c.Events = append(c.Events, e) }
func (c *Capture) Raise(r diagnostic.Raisable) { c.Emit(r.Diagnostic()) }
