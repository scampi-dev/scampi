// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"time"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/model"
	"scampi.dev/scampi/spec"
)

func Check(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) error {
	return forEachResolved(ctx, em, cfgPath, store, opts, func(ctx context.Context, e *Engine) error {
		return e.converge(ctx, true)
	})
}

func Apply(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) error {
	return forEachResolved(ctx, em, cfgPath, store, opts, func(ctx context.Context, e *Engine) error {
		return e.converge(ctx, false)
	})
}

func (e *Engine) Check(ctx context.Context) error { return e.converge(ctx, true) }
func (e *Engine) Apply(ctx context.Context) error { return e.converge(ctx, false) }

func (e *Engine) converge(ctx context.Context, checkOnly bool) error {
	start := time.Now()
	e.em.EmitEngineLifecycle(diagnostic.EngineStarted())

	p, _, hp, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return err
	}
	e.storeSourcePaths(ctx, p)

	var rep model.ExecutionReport
	var promisedPaths map[spec.Resource]bool
	if checkOnly {
		rep, promisedPaths, err = e.CheckPlan(ctx, p)
	} else {
		rep, err = e.ExecutePlan(ctx, p)
	}
	if err != nil {
		var cancelled CancelledError
		if errors.As(err, &cancelled) {
			e.em.EmitEngineLifecycle(diagnostic.EngineCancelled(rep, time.Since(start)))
		}
		return err
	}

	hookRep, err := e.executeHooks(ctx, rep, hp, checkOnly, promisedPaths)
	if err != nil {
		return err
	}
	hooksFired := len(hookRep.Actions)
	rep.Actions = append(rep.Actions, hookRep.Actions...)

	e.em.EmitEngineLifecycle(diagnostic.EngineFinished(rep, hooksFired, time.Since(start), err, checkOnly))

	return nil
}
