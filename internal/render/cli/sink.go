// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=stream
package cli

import (
	"fmt"
	"io"
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

// sink writes rendered lines synchronously. It needs no lock: the Emitter
// serializes the stream path and one-shot callers (plan/inspect/index) touch it
// from a single goroutine (see diagnostic.Output). No background goroutine and
// no live region - the TTY status line returns with typed progress in a later
// phase (#430).
type sink struct {
	out io.Writer
	err io.Writer
}

func newSink(out, err io.Writer) *sink {
	return &sink{out: out, err: err}
}

func (s *sink) emit(events []renderEvent) {
	for _, e := range events {
		w := s.out
		if e.stream == streamErr {
			w = s.err
		}
		_, _ = fmt.Fprintln(w, e.line)
	}
}
