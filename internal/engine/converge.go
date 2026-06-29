// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"sync"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/spec"
)

// Check resolves cfgPath into one execution per deploy, runs each in
// check-only mode, and returns the aggregated report across deploys.
// The cmd-side renders a summary line from the report.
func Check(
	ctx diagnostic.Ctx,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) (result.Execution, error) {
	return runConverge(ctx, cfgPath, store, opts, true)
}

// Apply resolves cfgPath into one execution per deploy, runs each in
// apply mode, and returns the aggregated report across deploys.
func Apply(
	ctx diagnostic.Ctx,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) (result.Execution, error) {
	return runConverge(ctx, cfgPath, store, opts, false)
}

// runConverge is the shared scaffolding for Check and Apply: iterate
// resolved deploys concurrently and aggregate their reports into one.
func runConverge(
	ctx diagnostic.Ctx,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
	checkOnly bool,
) (result.Execution, error) {
	var (
		mu  sync.Mutex
		agg result.Execution
	)
	err := forEachResolved(ctx, cfgPath, store, opts, func(ctx diagnostic.Ctx, e *Engine) error {
		rep, cErr := e.converge(ctx, checkOnly)
		if cErr != nil {
			return cErr
		}
		mu.Lock()
		agg.Steps = append(agg.Steps, rep.Steps...)
		mu.Unlock()
		return nil
	})
	return agg, err
}

// Check runs the deploy bound to this engine in check-only mode and
// returns its report.
func (e *Engine) Check(ctx diagnostic.Ctx) (result.Execution, error) {
	return e.converge(ctx, true)
}

// Apply runs the deploy bound to this engine and returns its report.
func (e *Engine) Apply(ctx diagnostic.Ctx) (result.Execution, error) {
	return e.converge(ctx, false)
}

func (e *Engine) converge(ctx diagnostic.Ctx, checkOnly bool) (result.Execution, error) {
	p, _, hp, err := plan(e.cfg, ctx, e.tgt.Capabilities())
	if err != nil {
		return result.Execution{}, err
	}
	e.storeSourcePaths(ctx, p)

	var rep result.Execution
	var promisedPaths map[spec.Resource]bool
	if checkOnly {
		rep, promisedPaths, err = e.CheckPlan(ctx, p)
	} else {
		rep, err = e.ExecutePlan(ctx, p)
	}
	if err != nil {
		return rep, err
	}

	hookRep, err := e.executeHooks(ctx, rep, hp, checkOnly, promisedPaths)
	if err != nil {
		return rep, err
	}
	rep.Steps = append(rep.Steps, hookRep.Steps...)
	return rep, nil
}
