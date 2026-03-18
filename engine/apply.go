// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"time"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/spec"
)

func Apply(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	opts spec.ResolveOptions,
) error {
	return runForEachResolved(ctx, em, cfgPath, store, opts, func(ctx context.Context, e *Engine) error {
		return e.Apply(ctx)
	})
}

func (e *Engine) Apply(ctx context.Context) error {
	start := time.Now()
	e.em.EmitEngineLifecycle(diagnostic.EngineStarted())

	p, _, hp, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return err
	}
	e.storeInputPaths(ctx, p)

	rep, err := e.ExecutePlan(ctx, p)
	if err != nil {
		var cancelled CancelledError
		if errors.As(err, &cancelled) {
			e.em.EmitEngineLifecycle(diagnostic.EngineCancelled(rep, time.Since(start)))
		}
		return err
	}

	hookRep, err := e.executeHooks(ctx, rep, hp, false, nil)
	if err != nil {
		return err
	}
	hooksFired := len(hookRep.Actions)
	rep.Actions = append(rep.Actions, hookRep.Actions...)

	e.em.EmitEngineLifecycle(diagnostic.EngineFinished(rep, hooksFired, time.Since(start), err, false))

	return nil
}
