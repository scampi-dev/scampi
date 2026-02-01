package local

import (
	"context"

	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

type Local struct{}

func (Local) Kind() string   { return "local" }
func (Local) NewConfig() any { return &Config{} }
func (Local) Create(_ context.Context, _ source.Source, _ spec.TargetInstance) (target.Target, error) {
	return POSIXTarget{}, nil
}

type Config struct{}
