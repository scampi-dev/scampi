// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"context"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
)

const collectorKey = "collector"

// Collector accumulates targets and deploy blocks during Starlark evaluation.
type Collector struct {
	ctx               context.Context
	path              string
	targets           map[string]spec.TargetInstance
	deploy            map[string]spec.DeployBlock
	src               source.Source
	files             map[string]*syntax.File
	secretsConfigured bool
}

func newCollector(ctx context.Context, path string, src source.Source) *Collector {
	return &Collector{
		ctx:     ctx,
		path:    path,
		targets: make(map[string]spec.TargetInstance),
		deploy:  make(map[string]spec.DeployBlock),
		src:     src,
		files:   make(map[string]*syntax.File),
	}
}

func threadCollector(thread *starlark.Thread) *Collector {
	v := thread.Local(collectorKey)
	c, ok := v.(*Collector)
	if !ok {
		panic(errs.BUG("thread %q: expected *Collector in thread-local %q, got %T", thread.Name, collectorKey, v))
	}
	return c
}

func (c *Collector) AddAST(name string, f *syntax.File) {
	c.files[name] = f
}

func (c *Collector) AST(name string) *syntax.File {
	return c.files[name]
}

// AddTarget registers a target instance. Returns an error if the name
// is already taken.
func (c *Collector) AddTarget(name string, inst spec.TargetInstance, span spec.SourceSpan) error {
	if _, exists := c.targets[name]; exists {
		return &DuplicateTargetError{Name: name, Source: span}
	}
	c.targets[name] = inst
	return nil
}

// AddDeploy registers a deploy block. Returns an error if the name
// is already taken.
func (c *Collector) AddDeploy(name string, block spec.DeployBlock, span spec.SourceSpan) error {
	if _, exists := c.deploy[name]; exists {
		return &DuplicateDeployError{Name: name, Source: span}
	}
	c.deploy[name] = block
	return nil
}

// SetSecretBackend wraps the collector's source with the given backend.
// Returns false if secrets() was already called.
func (c *Collector) SetSecretBackend(b secret.Backend) bool {
	if c.secretsConfigured {
		return false
	}
	c.src = source.WithSecrets(c.src, b)
	c.secretsConfigured = true
	return true
}

// Config drains the collector into a spec.Config.
func (c *Collector) Config() spec.Config {
	return spec.Config{
		Path:    c.path,
		Targets: c.targets,
		Deploy:  c.deploy,
	}
}
