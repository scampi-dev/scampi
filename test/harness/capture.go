// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
)

// Capture is an Output that buffers every event in arrival order, for tests
// that inspect what a producer raised. Embeds Discard for the one-shot no-ops;
// wrap it in diagnostic.NewEmitter and keep the reference to read Events.
type Capture struct {
	diagnostic.Discard
	Events []event.Event
}

func (c *Capture) RenderEvent(e event.Event) { c.Events = append(c.Events, e) }
