// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"fmt"
	"time"
)

type RunConfig struct {
	Dir          string
	ActionLogDir string
	Inventory    *Inventory // optional; loaded from ActionLogDir if nil
	Emitter      Emitter
	Interval     time.Duration
}

// Run keeps reconciling after a snapshot reject so operators can fix
// configs in place against the last-good snapshot.
func Run(ctx context.Context, cfg RunConfig) error {
	inv, actLog, err := openSinkAndInventory(cfg.ActionLogDir, cfg.Inventory)
	if err != nil {
		return err
	}
	defer func() { _ = actLog.Close() }()
	log := NewLog(FanoutEmitter{cfg.Emitter, actLog})
	dir, interval := cfg.Dir, cfg.Interval
	log.Info(ctx, "starting run loop", "dir", dir, "interval", interval)
	var (
		lastRev string
		snap    []Resource
	)
	bo := newBackoff()
	for {
		rev, hashErr := hashDir(dir)
		switch {
		case hashErr != nil:
			log.Error(ctx, "hash dir", "err", hashErr)
		case rev != lastRev:
			log.Debug(ctx, "snapshot change", "rev", rev)
			s, err := snapshot(ctx, dir, log)
			if err != nil {
				logReconcileErr(ctx, log, err)
			} else {
				snap = s
			}
			lastRev = rev
		}
		if snap != nil {
			tickStart := time.Now()
			tickOk := true
			reconcileRenames(ctx, snap, inv, log)
			orphans := append(inv.Orphans(snap), identityDrifts(snap, inv)...)
			if err := destroyAll(ctx, orphans, inv, log, bo); err != nil {
				logReconcileErr(ctx, log, fmt.Errorf("%w: %w", ErrReconcileFailed, err))
				tickOk = false
			}
			if err := applyAll(ctx, snap, inv, log, bo); err != nil {
				logReconcileErr(ctx, log, fmt.Errorf("%w: %w", ErrReconcileFailed, err))
				tickOk = false
			}
			status := "ok"
			if !tickOk {
				status = "failed"
			}
			log.Emit(
				ctx, CodeTickComplete, nil,
				"duration", time.Since(tickStart).Round(time.Millisecond).String(),
				"status", status,
			)
		}
		// Action log failure is fatal: persistence is broken, so we
		// stop reconciling rather than acting blind.
		if err := log.Err(); err != nil {
			return fmt.Errorf("action log: %w", err)
		}
		select {
		case <-ctx.Done():
			log.Info(ctx, "received shutdown signal, exiting at next safe point")
			return nil
		case <-time.After(interval):
		}
	}
}

// backoff tracks per-Ref retry deadlines. Methods are nil-safe.
type backoff struct {
	entries map[Ref]*backoffEntry
}

type backoffEntry struct {
	nextRetry time.Time
	attempts  int
}

func newBackoff() *backoff { return &backoff{entries: map[Ref]*backoffEntry{}} }

func (b *backoff) due(ref Ref, now time.Time) bool {
	if b == nil {
		return true
	}
	e, ok := b.entries[ref]
	if !ok {
		return true
	}
	return !now.Before(e.nextRetry)
}

func (b *backoff) success(ref Ref) {
	if b == nil {
		return
	}
	delete(b.entries, ref)
}

func (b *backoff) failure(ref Ref, now time.Time) {
	if b == nil {
		return
	}
	e, ok := b.entries[ref]
	if !ok {
		e = &backoffEntry{}
		b.entries[ref] = e
	}
	e.attempts++
	e.nextRetry = now.Add(backoffDelay(e.attempts))
}

// backoffDelay doubles per attempt starting at 1s, capped at 5 min.
func backoffDelay(attempts int) time.Duration {
	if attempts < 1 {
		return 0
	}
	shift := min(attempts-1, 30)
	d := time.Second << shift
	const maxDelay = 5 * time.Minute
	if d > maxDelay || d < 0 {
		return maxDelay
	}
	return d
}
