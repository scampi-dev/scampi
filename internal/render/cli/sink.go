// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=stream
package cli

import (
	"fmt"
	"io"

	"scampi.dev/scampi/internal/render/ansi"
)

type stream uint8

const (
	streamOut stream = iota
	streamErr
)

// renderEvent is one rendered line bound for stdout or stderr.
type renderEvent struct {
	line   string
	stream stream
	wrap   bool
}

// sink is the sole writer to the terminal and the owner of the live region: a
// block of ephemeral lines pinned to the bottom (in-flight steps + progress)
// that durable output scrolls above. It needs no lock: in stream mode the single
// consumer goroutine drives it; for one-shot output the Emitter serializes the
// caller. region is only drawn on a TTY; otherwise emit is a plain writer.
type sink struct {
	out io.Writer
	err io.Writer
	tty bool

	region []string // current live-region lines (already formatted + width-fit)
	drawn  int      // region lines currently on screen
}

func newSink(out, err io.Writer, tty bool) *sink {
	return &sink{out: out, err: err, tty: tty}
}

// emit writes durable lines: erase the live region, write the lines, then redraw
// the region beneath them so it stays pinned to the bottom.
func (s *sink) emit(events []renderEvent) {
	s.eraseRegion()
	for _, e := range events {
		w := s.out
		if e.stream == streamErr {
			w = s.err
		}
		_, _ = fmt.Fprintln(w, e.line)
	}
	s.drawRegion()
}

// setRegion replaces the live-region content and repaints it in place.
func (s *sink) setRegion(lines []string) {
	s.region = lines
	s.eraseRegion()
	s.drawRegion()
}

// clearRegion wipes the region for good (end of run), leaving only scrollback.
func (s *sink) clearRegion() {
	s.region = nil
	s.eraseRegion()
}

func (s *sink) eraseRegion() {
	if !s.tty || s.drawn == 0 {
		return
	}
	_, _ = fmt.Fprint(s.out, ansi.CursorUp(s.drawn)+ansi.EraseToEnd)
	s.drawn = 0
}

func (s *sink) drawRegion() {
	if !s.tty || len(s.region) == 0 {
		return
	}
	for _, l := range s.region {
		_, _ = fmt.Fprintln(s.out, l)
	}
	s.drawn = len(s.region)
}
