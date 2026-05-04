// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"scampi.dev/scampi/errs"
)

// Shell session pool
// -----------------------------------------------------------------------------
//
// The pool is two cooperating channels:
//
//   sem  — capacity = MaxSessions, holds tokens for "in-flight" sessions
//   free — capacity = MaxSessions, holds idle reusable shellSessions
//
// acquire() takes a sem token (blocks if max ops are already running),
// then either pops a free shell or opens a new one. release() pushes
// the shell back into free (or drops it if unhealthy) and returns the
// sem token. The combination caps concurrency at MaxSessions while
// reusing shells across as many ops as possible — a typical bench
// run goes from N-RTTs-per-op to 1-RTT-per-op.

type shellPool struct {
	sem      chan struct{}
	free     chan *shellSession
	open     func() (*shellSession, error)
	capacity int

	// counters — exposed via SSHTarget.Stats()
	sessionsOpened   atomic.Int64 // total shellSessions ever created
	sessionsInFlight atomic.Int64 // currently checked out
	sessionsPeakSeen atomic.Int64 // highwater for sessionsInFlight
	sessionRetries   atomic.Int64 // open() retries (server backpressure)
	commandsRun      atomic.Int64 // total commands executed across all sessions
}

func newShellPool(maxSessions int, open func() (*shellSession, error)) *shellPool {
	return &shellPool{
		sem:      make(chan struct{}, maxSessions),
		free:     make(chan *shellSession, maxSessions),
		open:     open,
		capacity: maxSessions,
	}
}

// acquire blocks until a shell is available or ctx is cancelled.
// Caller must call release exactly once with the returned shell.
//
// The loop interleaves "try free pool", "try open", and "wait" so a
// session that gets released mid-retry is picked up immediately
// instead of being missed because we were stuck in a backoff sleep.
// Server-side rejection on open triggers exponential backoff while
// staying responsive to releases.
func (p *shellPool) acquire(ctx context.Context) (*shellSession, error) {
	// Hold a slot for the whole duration — caps concurrency at p.max.
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	bo := sessionOpenBackoff()
	var lastOpenErr error

	for {
		// 1. Free shell ready? Take it.
		select {
		case s := <-p.free:
			p.markInFlight()
			return s, nil
		default:
		}

		// 2. Try to open a new shell.
		s, err := p.open()
		if err == nil {
			p.sessionsOpened.Add(1)
			p.markInFlight()
			return s, nil
		}
		p.sessionRetries.Add(1)
		lastOpenErr = err

		// 3. Backoff before next attempt — but stay responsive to
		// releases (a session might come back into free during the
		// wait, and we shouldn't miss it).
		next := bo.NextBackOff()
		if next == backoff.Stop {
			<-p.sem
			return nil, errs.WrapErrf(errSession, "%v", lastOpenErr)
		}
		timer := time.NewTimer(next)
		select {
		case s := <-p.free:
			timer.Stop()
			p.markInFlight()
			return s, nil
		case <-timer.C:
			// loop and try again
		case <-ctx.Done():
			timer.Stop()
			<-p.sem
			return nil, ctx.Err()
		}
	}
}

// release returns the shell to the pool (or drops it if unhealthy)
// and frees the in-flight token.
func (p *shellPool) release(s *shellSession) {
	p.sessionsInFlight.Add(-1)
	if s.healthy() {
		select {
		case p.free <- s:
			// pooled
		default:
			// pool unexpectedly full — close to avoid leak
			_ = s.close()
		}
	} else {
		_ = s.close()
	}
	<-p.sem
}

// closeAll drains the free pool and closes every shell. Called from
// SSHTarget.Close. Sessions still in-flight are not touched here —
// they'll be closed by their final release.
func (p *shellPool) closeAll() {
	for {
		select {
		case s := <-p.free:
			_ = s.close()
		default:
			return
		}
	}
}

// markInFlight increments the in-flight count and updates the peak.
func (p *shellPool) markInFlight() {
	now := p.sessionsInFlight.Add(1)
	for {
		peak := p.sessionsPeakSeen.Load()
		if now <= peak || p.sessionsPeakSeen.CompareAndSwap(peak, now) {
			break
		}
	}
}

// sessionOpenBackoff returns the standard exponential-backoff
// schedule used by the pool. Aggressive initial wait absorbs the
// SFTP-vs-MaxSessions race; capped per-attempt and total.
func sessionOpenBackoff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 10 * time.Millisecond
	b.MaxInterval = 1 * time.Second
	b.Multiplier = 2.0
	b.RandomizationFactor = 0.5
	b.MaxElapsedTime = 5 * time.Second
	b.Reset()
	return b
}
