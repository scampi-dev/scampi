// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"time"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/spec"
)

func Check(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	opts spec.ResolveOptions,
) error {
	return runForEachResolved(ctx, em, cfgPath, store, opts, func(ctx context.Context, e *Engine) error {
		return e.Check(ctx)
	})
}

func (e *Engine) Check(ctx context.Context) error {
	start := time.Now()
	e.em.EmitEngineLifecycle(diagnostic.EngineStarted())

	plan, _, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return err
	}
	e.storeInputPaths(ctx, plan)

	rep, err := e.CheckPlan(ctx, plan)
	if err != nil {
		return err
	}

	e.em.EmitEngineLifecycle(diagnostic.EngineFinished(rep, time.Since(start), err, true))

	return nil
}
