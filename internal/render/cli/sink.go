// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=stream
package cli

import (
	"fmt"
	"io"
	"sync"
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

// sink writes rendered lines synchronously. A mutex serializes the
// concurrent Emit calls the engine makes from parallel op/action goroutines
// during apply; one-shot callers (plan/inspect/index) touch it from a single
// goroutine. No background goroutine and no live region - the TTY status line
// returns with typed progress in a later phase (#430).
type sink struct {
	mu  sync.Mutex
	out io.Writer
	err io.Writer
}

func newSink(out, err io.Writer) *sink {
	return &sink{out: out, err: err}
}

func (s *sink) emit(events []renderEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range events {
		w := s.out
		if e.stream == streamErr {
			w = s.err
		}
		_, _ = fmt.Fprintln(w, e.line)
	}
}
