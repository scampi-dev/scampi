package engine

import (
	"context"
	"time"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
)

func Apply(ctx context.Context, em diagnostic.Emitter, cfgPath string, store *spec.SourceStore) error {
	src := source.WithRoot(cfgPath, source.LocalPosixSource{})
	cfg, err := LoadConfig(ctx, em, cfgPath, store, src)
	if err != nil {
		return err
	}

	e, err := New(ctx, src, cfg, em)
	if err != nil {
		return err
	}
	defer e.Close()

	return e.Apply(ctx)
}

func (e *Engine) Apply(ctx context.Context) error {
	start := time.Now()
	e.em.EmitEngineLifecycle(diagnostic.EngineStarted())

	plan, _, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return err
	}

	rep, err := e.ExecutePlan(ctx, plan)
	if err != nil {
		// fail-fast preserved
		return err
	}

	e.em.EmitEngineLifecycle(diagnostic.EngineFinished(rep, time.Since(start), err, false))

	return nil
}
