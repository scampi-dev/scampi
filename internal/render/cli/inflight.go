// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"sort"
	"time"

	"scampi.dev/scampi/internal/diagnostic/event"
)

// inflight tracks, per deploy lane, the steps that have begun but not finished,
// plus how many have finished. The live region reads it to draw the in-flight
// set and per-deploy progress. It is fed the raw event stream (begin/finish in
// real completion order), not the order-buffered durable one, so "what is
// running now" is accurate.
//
// Not concurrency-safe, and it doesn't need to be: it lives inside the single
// stream-consumer goroutine, which is the only thing that touches it (and the
// only writer to the terminal). The channel feeding that consumer is the
// serialization point.
type inflight struct {
	lanes map[int]*laneState // by deploy ordinal
	order []int              // ordinals in first-seen order, for stable display
}

type laneState struct {
	name     string
	running  []runningStep // begun, not finished, in begin order
	finished int
}

type runningStep struct {
	ref     event.StepRef
	started time.Time
}

func newInflight() *inflight {
	return &inflight{lanes: map[int]*laneState{}}
}

// begin records a step entering execution.
func (f *inflight) begin(s event.StepRef, now time.Time) {
	l := f.lane(s.Deploy)
	l.running = append(l.running, runningStep{ref: s, started: now})
}

// finish records a step settling: it leaves the running set and bumps the
// lane's finished count.
func (f *inflight) finish(s event.StepRef) {
	l := f.lane(s.Deploy)
	for i, r := range l.running {
		if r.ref.Index == s.Index {
			l.running = append(l.running[:i], l.running[i+1:]...)
			break
		}
	}
	l.finished++
}

// anyRunning reports whether any lane has an in-flight step.
func (f *inflight) anyRunning() bool {
	for _, l := range f.lanes {
		if len(l.running) > 0 {
			return true
		}
	}
	return false
}

// laneView is a read-only per-lane snapshot for the live region. Running is
// ordered longest-running first.
type laneView struct {
	Name     string
	Running  []runningStep
	Finished int
}

// view returns the lanes in first-seen order, each lane's running steps sorted
// longest-running first (oldest start time), for the live region to render.
func (f *inflight) view() []laneView {
	out := make([]laneView, 0, len(f.order))
	for _, ord := range f.order {
		l := f.lanes[ord]
		running := append([]runningStep(nil), l.running...)
		sort.SliceStable(running, func(i, j int) bool {
			return running[i].started.Before(running[j].started)
		})
		out = append(out, laneView{Name: l.name, Running: running, Finished: l.finished})
	}
	return out
}

func (f *inflight) lane(d event.DeployRef) *laneState {
	l := f.lanes[d.Ordinal]
	if l == nil {
		l = &laneState{name: d.Name}
		f.lanes[d.Ordinal] = l
		f.order = append(f.order, d.Ordinal)
	}
	return l
}
