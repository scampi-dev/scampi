// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=stream
package cli

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"scampi.dev/scampi/render/ansi"
)

type stream uint8

const (
	streamOut stream = iota
	streamErr
)

// renderEvent is either a permanent line or a live region mutation.
// If live is non-nil, the event is a live update (no line is printed).
type renderEvent struct {
	line   string
	stream stream
	wrap   bool
	live   *liveUpdate
}

type liveUpdateKind uint8

const (
	liveActionStarted liveUpdateKind = iota
	liveActionFinished
)

type liveUpdate struct {
	kind liveUpdateKind
	id   string
	desc string
}

type activeAction struct {
	id   string
	desc string
}

const (
	spinnerInterval    = 80 * time.Millisecond
	maxVisibleSpinners = 5
)

type renderer struct {
	out  io.Writer
	err  io.Writer
	ch   chan renderEvent
	done chan struct{}

	isTTY     bool
	glyphs    glyphSet
	formatter *formatter

	interrupted   atomic.Bool
	active        []activeAction
	spinnerTick   int
	lastLiveLines int
}

func newRenderer(out, err io.Writer, isTTY bool, glyphs glyphSet, fmt *formatter) *renderer {
	r := &renderer{
		out:       out,
		err:       err,
		ch:        make(chan renderEvent, 256),
		done:      make(chan struct{}),
		isTTY:     isTTY,
		glyphs:    glyphs,
		formatter: fmt,
	}

	go r.loop()
	return r
}

func (r *renderer) loop() {
	var ticker *time.Ticker
	var tickCh <-chan time.Time

	if r.isTTY {
		ticker = time.NewTicker(spinnerInterval)
		tickCh = ticker.C
	}

	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
		close(r.done)
	}()

	for {
		select {
		case e, ok := <-r.ch:
			if !ok {
				r.clearLive()
				return
			}
			if e.live != nil {
				r.applyLiveUpdate(*e.live)
				continue
			}
			r.clearLive()
			w := r.out
			if e.stream == streamErr {
				w = r.err
			}
			_, _ = fmt.Fprintln(w, e.line)
			r.drawLive()

		case <-tickCh:
			r.spinnerTick++
			r.drawLive()
		}
	}
}

func (r *renderer) applyLiveUpdate(u liveUpdate) {
	switch u.kind {
	case liveActionStarted:
		r.clearLive()
		r.active = append(r.active, activeAction{id: u.id, desc: u.desc})
		r.drawLive()
	case liveActionFinished:
		r.clearLive()
		r.removeAction(u.id)
		// Don't drawLive here — the next permanent event (action summary)
		// follows immediately and will redraw after printing.
	}
}

func (r *renderer) removeAction(id string) {
	for i, a := range r.active {
		if a.id == id {
			r.active = append(r.active[:i], r.active[i+1:]...)
			return
		}
	}
}

// clearLive erases the spinner region and leaves the cursor at the
// beginning of where the first spinner line was.
func (r *renderer) clearLive() {
	if !r.isTTY || r.lastLiveLines == 0 {
		return
	}
	// Cursor is at the beginning of the line after the last spinner.
	// Move up to the first spinner line, then erase everything below.
	_, _ = fmt.Fprint(r.out, ansi.CursorUp(r.lastLiveLines)+ansi.EraseToEnd)
	r.lastLiveLines = 0
}

// drawLive renders the spinner block. After return the cursor sits at
// the beginning of the line after the last spinner line.
func (r *renderer) drawLive() {
	if !r.isTTY || len(r.active) == 0 {
		return
	}

	// Return to the top of the existing live region.
	if r.lastLiveLines > 0 {
		_, _ = fmt.Fprint(r.out, ansi.CursorUp(r.lastLiveLines))
	}
	_, _ = fmt.Fprint(r.out, ansi.EraseToEnd)

	frames := r.glyphs.spinnerFrames
	frame := frames[r.spinnerTick%len(frames)]

	visible := r.active
	overflow := 0
	if len(visible) > maxVisibleSpinners {
		overflow = len(visible) - maxVisibleSpinners
		visible = visible[:maxVisibleSpinners]
	}

	interrupted := r.interrupted.Load()

	lines := 0
	for _, a := range visible {
		line := r.formatter.fmtfMsg(colSpinner, "%s [%s]", frame, a.id)
		if a.desc != "" {
			line += r.formatter.fmtfMsg(colSpinner, " %s", a.desc)
		}
		if interrupted {
			line += r.formatter.fmtfMsg(colSpinner, " — interrupted, finishing...")
		}
		_, _ = fmt.Fprintln(r.out, line+ansi.Reset)
		lines++
	}

	if overflow > 0 {
		msg := r.formatter.fmtfMsg(colSpinner,
			"  ... and %d more action%s", overflow, plural(overflow))
		_, _ = fmt.Fprintln(r.out, msg+ansi.Reset)
		lines++
	}

	r.lastLiveLines = lines
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (r *renderer) close() {
	close(r.ch)
	<-r.done
}

func (r *renderer) emitEvents(events []renderEvent) {
	for _, e := range events {
		select {
		case r.ch <- e:
		case <-r.done:
		}
	}
}

func (r *renderer) sendLive(u liveUpdate) {
	select {
	case r.ch <- renderEvent{live: &u}:
	case <-r.done:
	}
}
