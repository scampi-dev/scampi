package engine

import (
	"context"
	"path/filepath"
	"time"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

func Check(ctx context.Context, em diagnostic.Emitter, cfgPath string, store *spec.SourceStore) error {
	e := New(
		source.LocalPosixSource{},
		target.LocalPosixTarget{}, // Need target for Check reads
		em,
	)

	return e.Check(ctx, cfgPath, store)
}

func (e *Engine) Check(ctx context.Context, cfgPath string, store *spec.SourceStore) error {
	start := time.Now()
	e.em.EmitEngineLifecycle(diagnostic.EngineStarted())

	cfgPath, err := filepath.Abs(cfgPath)
	if err != nil {
		panic(errs.BUG("filepath.Abs() failed: %w", err))
	}

	cfg, err := LoadConfigWithSource(ctx, e.em, cfgPath, store, e.src)
	if err != nil {
		return err
	}

	plan, err := plan(cfg, e.em)
	if err != nil {
		return err
	}

	rep, err := e.CheckPlan(ctx, plan)
	if err != nil {
		// fail-fast preserved
		return err
	}

	e.em.EmitEngineLifecycle(diagnostic.EngineFinished(rep, time.Since(start), err, true))

	return nil
}
