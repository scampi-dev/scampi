// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

type Engine struct {
	src    source.Source
	tgt    target.Target
	cfg    spec.Config
	store  *diagnostic.SourceStore
	deploy event.DeployRef // lane identity for events this engine emits
}

func New(ctx diagnostic.Ctx, src source.Source, cfg spec.Config) (*Engine, error) {
	tgt, err := cfg.Target.Type.Create(ctx, src, cfg.Target)
	if err != nil {
		if impact, ok := emitEngineDiagnostic(ctx, cfg.Path, err); ok {
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
	}, nil
}

// NewWithTarget creates an engine with a pre-created target.
// Use this for testing when you need to provide a specific target instance.
func NewWithTarget(
	_ diagnostic.Ctx,
	src source.Source,
	cfg spec.Config,
	tgt target.Target,
) (*Engine, error) {
	return &Engine{
		src: src,
		tgt: tgt,
		cfg: cfg,
	}, nil
}

func (e *Engine) Close() {
	if closer, ok := e.tgt.(target.Closer); ok {
		closer.Close()
	}
}

// storeSourcePaths reads step source files (e.g. template sources) via the
// source and registers them in the store so the renderer can display source
// context in error messages.
func (e *Engine) storeSourcePaths(ctx diagnostic.Ctx, p spec.Plan) {
	if e.store == nil {
		return
	}
	for _, act := range p.Deploy.Steps {
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
	ctx diagnostic.Ctx,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
	run func(ctx diagnostic.Ctx, e *Engine) error,
) error {
	src := source.WithRoot(cfgPath, source.LocalPosixSource{})
	cfg, err := LoadConfig(ctx, cfgPath, store, src)
	if err != nil {
		return err
	}

	resolved, err := ResolveMultiple(cfg, opts)
	if err != nil {
		if impact, ok := emitEngineDiagnostic(ctx, cfgPath, err); ok {
			if impact.ShouldAbort() {
				return AbortError{Causes: []error{err}}
			}
		}
		return err
	}

	allCaps := capabilityTarget{caps: capability.All}
	return runPlansConcurrent(ctx, resolved, func(ctx diagnostic.Ctx, dr event.DeployRef, res spec.Config) error {
		e, err := NewWithTarget(ctx, src, res, allCaps)
		if err != nil {
			return err
		}
		defer e.Close()
		e.store = store
		e.deploy = dr
		return run(ctx, e)
	})
}

func forEachResolved(
	ctx diagnostic.Ctx,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
	run func(ctx diagnostic.Ctx, e *Engine) error,
) error {
	src := source.WithRoot(cfgPath, source.LocalPosixSource{})
	cfg, err := LoadConfig(ctx, cfgPath, store, src)
	if err != nil {
		return err
	}

	resolved, err := ResolveMultiple(cfg, opts)
	if err != nil {
		if impact, ok := emitEngineDiagnostic(ctx, cfgPath, err); ok {
			if impact.ShouldAbort() {
				return AbortError{Causes: []error{err}}
			}
		}
		return err
	}

	return runPlansConcurrent(ctx, resolved, func(ctx diagnostic.Ctx, dr event.DeployRef, res spec.Config) error {
		e, err := New(ctx, src, res)
		if err != nil {
			return err
		}
		defer e.Close()
		e.store = store
		e.deploy = dr
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
	ctx diagnostic.Ctx,
	resolved []spec.Config,
	work func(ctx diagnostic.Ctx, dr event.DeployRef, res spec.Config) error,
) error {
	if len(resolved) == 1 {
		// Single lane: empty Name keeps the output untagged.
		return work(ctx, event.DeployRef{}, resolved[0])
	}

	graph, err := buildDeployGraph(resolved)
	if err != nil {
		return err
	}

	// Assign lane ordinals level-major, declaration order within a level, so
	// every deploy has a stable identity for the render Sequencer's per-deploy
	// cursor and for tagging. nameW is the widest lane name, so the renderer can
	// pad tags and keep the step indexes aligned across lanes.
	ordOf := make(map[*deployNode]int)
	ord, nameW := 0, 0
	for _, level := range graph.levels {
		nodes := append([]*deployNode(nil), level...)
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].idx < nodes[j].idx })
		for _, n := range nodes {
			ordOf[n] = ord
			ord++
			if w := len(n.res.DeployName); w > nameW {
				nameW = w
			}
		}
	}

	var causes []error
	for _, level := range graph.levels {
		if len(causes) > 0 {
			// Upstream level produced failures — skip downstream nodes
			// rather than racing them into ops that depend on
			// resources the failed producer was supposed to create.
			break
		}
		levelCauses := runLevel(ctx, level, ordOf, nameW, work)
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
	ctx diagnostic.Ctx,
	level []*deployNode,
	ordOf map[*deployNode]int,
	nameW int,
	work func(ctx diagnostic.Ctx, dr event.DeployRef, res spec.Config) error,
) []error {
	g, gctx := errgroup.WithContext(ctx)
	var (
		mu     sync.Mutex
		causes []error
	)
	for _, n := range level {
		dr := event.DeployRef{Name: n.res.DeployName, Ordinal: ordOf[n], MaxNameWidth: nameW}
		g.Go(func() error {
			if err := work(ctx.With(gctx), dr, n.res); err != nil {
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
