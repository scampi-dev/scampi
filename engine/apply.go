package engine

import (
	"context"
	"path/filepath"
	"time"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"godoit.dev/doit/util"
)

func Apply(ctx context.Context, em diagnostic.Emitter, cfgPath string, store *spec.SourceStore) error {
	e := New(
		source.LocalPosixSource{},
		target.LocalPosixTarget{},
		em,
	)

	return e.Apply(ctx, cfgPath, store)
}

func (e *Engine) Apply(ctx context.Context, cfgPath string, store *spec.SourceStore) error {
	start := time.Now()
	e.em.Emit(diagnostic.EngineStarted())

	cfgPath, err := filepath.Abs(cfgPath)
	if err != nil {
		panic(util.BUG("filepath.Abs() failed: %w", err))
	}

	cfg, err := LoadConfigWithSource(e.em, cfgPath, store, e.src)
	if err != nil {
		return err
	}

	plan, err := plan(cfg, e.em)
	if err != nil {
		return err
	}

	results, err := e.ExecutePlan(ctx, plan)
	if err != nil {
		return err
	}

	rs := diagnostic.RunSummary{
		ChangedCount: 0,
		FailedCount:  0,
		TotalCount:   len(results),
	}
	for _, res := range results {
		if res.Res.Changed {
			rs.ChangedCount++
		}
		if res.Err != nil {
			rs.FailedCount++
		}
	}

	e.em.Emit(diagnostic.EngineFinished(rs, time.Since(start), err))

	return err
}
