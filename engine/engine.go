// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

type Engine struct {
	src   source.Source
	tgt   target.Target
	cfg   spec.ResolvedConfig
	em    diagnostic.Emitter
	store *diagnostic.SourceStore
}

func New(ctx context.Context, src source.Source, cfg spec.ResolvedConfig, em diagnostic.Emitter) (*Engine, error) {
	em.EmitEngineLifecycle(diagnostic.EngineConnecting(cfg.TargetName, cfg.Target.Type.Kind()))
	tgt, err := cfg.Target.Type.Create(ctx, src, cfg.Target)
	if err != nil {
		if impact, ok := emitEngineDiagnostic(em, cfg.Path, err); ok {
			if impact.ShouldAbort() {
				return nil, AbortError{Causes: []error{err}}
			}
		}
		return nil, err
	}

	return &Engine{
		src: src,
		tgt: tgt,
		cfg: cfg,
		em:  em,
	}, nil
}

// NewWithTarget creates an engine with a pre-created target.
// Use this for testing when you need to provide a specific target instance.
func NewWithTarget(
	_ context.Context,
	src source.Source,
	cfg spec.ResolvedConfig,
	em diagnostic.Emitter,
	tgt target.Target,
) (*Engine, error) {
	return &Engine{
		src: src,
		tgt: tgt,
		cfg: cfg,
		em:  em,
	}, nil
}

func (e *Engine) Close() {
	if closer, ok := e.tgt.(target.Closer); ok {
		closer.Close()
	}
}

// storeSourcePaths reads action source files (e.g. template sources) via the
// source and registers them in the store so the renderer can display source
// context in error messages.
func (e *Engine) storeSourcePaths(ctx context.Context, p spec.Plan) {
	if e.store == nil {
		return
	}
	for _, act := range p.Unit.Actions {
		sr, ok := act.(spec.SourceReader)
		if !ok {
			continue
		}
		for _, path := range sr.SourcePaths() {
			if data, err := e.src.ReadFile(ctx, path); err == nil {
				e.store.AddFile(path, data)
			}
		}
	}
}

// forEachResolvedOffline is like forEachResolved but skips target creation.
// Used by plan and inspect (list mode) which never touch the target.
func forEachResolvedOffline(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
	run func(ctx context.Context, e *Engine) error,
) error {
	src := source.WithRoot(cfgPath, source.LocalPosixSource{})
	cfg, err := LoadConfig(ctx, em, cfgPath, store, src)
	if err != nil {
		return err
	}

	resolved, err := ResolveMultiple(cfg, opts)
	if err != nil {
		if impact, ok := emitEngineDiagnostic(em, cfgPath, err); ok {
			if impact.ShouldAbort() {
				return AbortError{Causes: []error{err}}
			}
		}
		return err
	}

	allCaps := capabilityTarget{caps: capability.All}
	return runPlansConcurrent(ctx, em, resolved, func(ctx context.Context, res spec.ResolvedConfig) error {
		e, err := NewWithTarget(ctx, src, res, em, allCaps)
		if err != nil {
			return err
		}
		defer e.Close()
		e.store = store
		return run(ctx, e)
	})
}

func forEachResolved(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
	run func(ctx context.Context, e *Engine) error,
) error {
	src := source.WithRoot(cfgPath, source.LocalPosixSource{})
	cfg, err := LoadConfig(ctx, em, cfgPath, store, src)
	if err != nil {
		return err
	}

	resolved, err := ResolveMultiple(cfg, opts)
	if err != nil {
		if impact, ok := emitEngineDiagnostic(em, cfgPath, err); ok {
			if impact.ShouldAbort() {
				return AbortError{Causes: []error{err}}
			}
		}
		return err
	}

	return runPlansConcurrent(ctx, em, resolved, func(ctx context.Context, res spec.ResolvedConfig) error {
		e, err := New(ctx, src, res, em)
		if err != nil {
			return err
		}
		defer e.Close()
		e.store = store
		return run(ctx, e)
	})
}

// runPlansConcurrent runs resolved plans level-by-level according to
// the cross-deploy resource graph: nodes within a level run in
// parallel, level boundaries enforce ordering. A failure in one plan
// does not cancel siblings within the level, but downstream levels
// inherit the failure (their producers couldn't satisfy them, so they
// abort). ctx cancellation (e.g. SIGINT) propagates to all in-flight
// plans. See #275.
//
// Concurrency within a level is unbounded — see #236 for the
// rationale and follow-ups for a tunable cap if real configs hit
// shared-infra limits.
func runPlansConcurrent(
	ctx context.Context,
	em diagnostic.Emitter,
	resolved []spec.ResolvedConfig,
	work func(ctx context.Context, res spec.ResolvedConfig) error,
) error {
	if len(resolved) == 1 {
		return work(ctx, resolved[0])
	}

	graph, err := buildDeployGraph(resolved)
	if err != nil {
		return err
	}
	emitGraphSummary(em, graph)

	var causes []error
	for _, level := range graph.levels {
		if len(causes) > 0 {
			// Upstream level produced failures — skip downstream nodes
			// rather than racing them into ops that depend on
			// resources the failed producer was supposed to create.
			break
		}
		levelCauses := runLevel(ctx, level, work)
		causes = append(causes, levelCauses...)
	}

	if len(causes) == 1 {
		return causes[0]
	}
	if len(causes) > 0 {
		return AbortError{Causes: causes}
	}
	return nil
}

func runLevel(
	ctx context.Context,
	level []*deployNode,
	work func(ctx context.Context, res spec.ResolvedConfig) error,
) []error {
	g, gctx := errgroup.WithContext(ctx)
	var (
		mu     sync.Mutex
		causes []error
	)
	for _, n := range level {
		g.Go(func() error {
			if err := work(gctx, n.res); err != nil {
				mu.Lock()
				causes = append(causes, err)
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	return causes
}
