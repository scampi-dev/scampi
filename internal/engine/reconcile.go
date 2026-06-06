// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"fmt"
)

type ReconcileConfig struct {
	Dir          string
	ActionLogDir string
	Inventory    *Inventory // optional; loaded from ActionLogDir if nil
	Emitter      Emitter
}

func Reconcile(ctx context.Context, cfg ReconcileConfig) error {
	inv, actLog, err := openSinkAndInventory(cfg.ActionLogDir, cfg.Inventory)
	if err != nil {
		return err
	}
	defer func() { _ = actLog.Close() }()
	log := NewLog(FanoutEmitter{cfg.Emitter, actLog})

	snap, err := snapshot(ctx, cfg.Dir, log)
	if err != nil {
		return err
	}
	var errs []error
	reconcileRenames(ctx, snap, inv, log)
	orphans := append(inv.Orphans(snap), identityDrifts(snap, inv)...)
	if err := destroyAll(ctx, orphans, inv, log, nil); err != nil {
		errs = append(errs, err)
	}
	if err := applyAll(ctx, snap, inv, log, nil); err != nil {
		errs = append(errs, err)
	}
	if err := log.Err(); err != nil {
		errs = append(errs, fmt.Errorf("action log: %w", err))
	}
	if ctx.Err() != nil {
		log.Info(ctx, "received shutdown signal, exiting at next safe point")
		return ctx.Err()
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrReconcileFailed, errors.Join(errs...))
	}
	return nil
}
