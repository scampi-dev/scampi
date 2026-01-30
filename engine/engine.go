package engine

import (
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

type Engine struct {
	src source.Source
	tgt target.Target
	cfg spec.Config
	em  diagnostic.Emitter
}

func New(src source.Source, cfg spec.Config, em diagnostic.Emitter) (*Engine, error) {
	tgt, err := cfg.Target.Type.Create(cfg.Target.Config)
	if err != nil {
		return nil, err
	}

	return &Engine{
		src: src,
		tgt: tgt,
		cfg: cfg,
		em:  em,
	}, nil
}
