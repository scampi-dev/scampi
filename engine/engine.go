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
	src source.Source
	tgt target.Target
	cfg spec.ResolvedConfig
	em  diagnostic.Emitter
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

		if err := run(ctx, e); err != nil {
			e.Close()
			return err
		}
		e.Close()
	}

	return nil
}
