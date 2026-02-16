// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

type Engine struct {
	src   source.Source
	tgt   target.Target
	cfg   spec.ResolvedConfig
	em    diagnostic.Emitter
	store *spec.SourceStore
}

func New(ctx context.Context, src source.Source, cfg spec.ResolvedConfig, em diagnostic.Emitter) (*Engine, error) {
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

// storeInputPaths reads action input files (e.g. template sources) via the
// source and registers them in the store so the renderer can display source
// context in error messages.
func (e *Engine) storeInputPaths(ctx context.Context, p spec.Plan) {
	if e.store == nil {
		return
	}
	for _, act := range p.Unit.Actions {
		pather, ok := act.(spec.Pather)
		if !ok {
			continue
		}
		for _, path := range pather.InputPaths() {
			if data, err := e.src.ReadFile(ctx, path); err == nil {
				e.store.AddFile(path, data)
			}
		}
	}
}

func runForEachResolved(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
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

	for _, res := range resolved {
		e, err := New(ctx, src, res, em)
		if err != nil {
			return err
		}
		e.store = store

		if err := run(ctx, e); err != nil {
			e.Close()
			return err
		}
		e.Close()
	}

	return nil
}
