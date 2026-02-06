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

type renderEvent struct {
	line   string
	stream stream
}

type renderer struct {
	out  io.Writer
	err  io.Writer
	ch   chan renderEvent
	done chan struct{}
}

func newRenderer(out, err io.Writer) *renderer {
	r := &renderer{
		out:  out,
		err:  err,
		ch:   make(chan renderEvent, 256),
		done: make(chan struct{}),
	}

	go func() {
		for e := range r.ch {
			w := r.out
			if e.stream == streamErr {
				w = r.err
			}
			_, _ = fmt.Fprintln(w, e.line)
		}
		close(r.done)
	}()
	return r
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
