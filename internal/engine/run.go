// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"fmt"
	"time"

	"scampi.dev/scampi/internal/mesh"
)

type RunConfig struct {
	Dir          string
	ActionLogDir string
	Inventory    *Inventory // optional; loaded from ActionLogDir if nil
	Emitter      Emitter
	Interval     time.Duration
	Mesh         *MeshConfig // optional; nil disables mesh
}

// MeshConfig carries the per-peer mesh settings that vary by
// deployment. Zero LeaveTimeout / SnapshotDebounce pick sensible
// defaults; everything else is required when Mesh is non-nil.
type MeshConfig struct {
	Name             string
	BindAddr         string
	BindPort         int
	AdvertiseAddr    string
	AdvertisePort    int
	Join             []string
	SnapshotPath     string
	SnapshotDebounce time.Duration
	LeaveTimeout     time.Duration
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

	if cfg.Mesh != nil {
		if stop := startMesh(ctx, cfg.Mesh, cfg.Emitter, log); stop != nil {
			defer stop()
		}
	}

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

const (
	defaultMeshLeaveTimeout     = 2 * time.Second
	defaultMeshSnapshotDebounce = 1 * time.Second
)

// startMesh brings up the mesh substrate. Failures are non-fatal:
// engine emits CodeMeshUnavailable and keeps reconciling. Returns
// a cleanup func when mesh did come up; nil otherwise.
func startMesh(ctx context.Context, cfg *MeshConfig, emitter Emitter, log Log) func() {
	debounce := cfg.SnapshotDebounce
	if debounce == 0 {
		debounce = defaultMeshSnapshotDebounce
	}
	leave := cfg.LeaveTimeout
	if leave == 0 {
		leave = defaultMeshLeaveTimeout
	}
	m, err := mesh.Run(ctx, mesh.Config{
		Name:             cfg.Name,
		BindAddr:         cfg.BindAddr,
		BindPort:         cfg.BindPort,
		AdvertiseAddr:    cfg.AdvertiseAddr,
		AdvertisePort:    cfg.AdvertisePort,
		Join:             cfg.Join,
		SnapshotPath:     cfg.SnapshotPath,
		Logger:           log,
		SnapshotDebounce: debounce,
	})
	if err != nil {
		emitter.Emit(ctx, CodeMeshUnavailable, nil, "err", err.Error())
		return nil
	}
	emitter.Emit(
		ctx, CodeMeshUp, nil,
		"name", m.Self().Name,
		"addr", m.Self().Addr,
		"members", len(m.Members()),
	)
	go forwardMeshEvents(ctx, emitter, m)
	return func() {
		_ = m.Leave(leave)
		_ = m.Shutdown()
		emitter.Emit(ctx, CodeMeshDown, nil)
	}
}

func forwardMeshEvents(ctx context.Context, emitter Emitter, m *mesh.Mesh) {
	for ev := range m.Events() {
		var code Code
		switch ev.Kind {
		case mesh.EventJoin:
			code = CodeMeshPeerJoined
		case mesh.EventLeave:
			code = CodeMeshPeerLeft
		case mesh.EventUpdate:
			code = CodeMeshPeerUpdated
		default:
			continue
		}
		emitter.Emit(ctx, code, nil, "name", ev.Peer.Name, "addr", ev.Peer.Addr)
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
