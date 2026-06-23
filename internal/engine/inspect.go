// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

// InspectDiffResult holds the content pair for diff mode.
type InspectDiffResult struct {
	DestPath string
	Current  []byte // nil when the file does not yet exist on target
	Desired  []byte
}

// InspectList resolves each deploy and returns one InspectDetail per
// successfully resolved deploy. Errors from any single deploy abort
// the iteration. Results are sorted by DeployName so the caller
// renders deterministically; per-deploy resolution runs concurrently.
func InspectList(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) ([]event.InspectDetail, error) {
	var (
		mu      sync.Mutex
		details []event.InspectDetail
	)
	err := forEachResolvedOffline(ctx, em, cfgPath, store, opts, func(ctx context.Context, e *Engine) error {
		d, err := e.buildInspect(ctx)
		if err != nil {
			return err
		}
		mu.Lock()
		details = append(details, d)
		mu.Unlock()
		return nil
	})
	slices.SortStableFunc(details, func(a, b event.InspectDetail) int {
		return strings.Compare(a.DeployName, b.DeployName)
	})
	return details, err
}

// InspectDiffPaths returns destination paths of all diffable ops.
func InspectDiffPaths(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) ([]string, error) {
	var (
		mu    sync.Mutex
		paths []string
	)

	err := forEachResolvedOffline(ctx, em, cfgPath, store, opts, func(_ context.Context, e *Engine) error {
		p, _, _, planErr := plan(e.cfg, e.em, e.tgt.Capabilities())
		if planErr != nil {
			return planErr
		}
		var local []string
		for _, act := range p.Deploy.Actions {
			for _, op := range act.Ops() {
				if d, ok := op.(spec.Diffable); ok {
					local = append(local, d.DestPath())
				}
			}
		}
		mu.Lock()
		paths = append(paths, local...)
		mu.Unlock()
		return nil
	})

	return paths, err
}

// InspectDiff returns desired vs current content for a specific file op (diff mode).
func InspectDiff(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
	destPath string,
) (*InspectDiffResult, error) {
	var (
		mu     sync.Mutex
		result *InspectDiffResult
	)

	err := forEachResolved(ctx, em, cfgPath, store, opts, func(ctx context.Context, e *Engine) error {
		r, err := e.InspectDiffFile(ctx, destPath)
		if err != nil {
			return err
		}
		mu.Lock()
		result = r
		mu.Unlock()
		return nil
	})

	return result, err
}

func (e *Engine) buildInspect(ctx context.Context) (event.InspectDetail, error) {
	p, _, _, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return event.InspectDetail{}, err
	}
	e.storeSourcePaths(ctx, p)

	detail := event.InspectDetail{
		DeployName: e.cfg.DeployName,
		TargetName: e.cfg.TargetName,
	}
	for i, act := range p.Deploy.Actions {
		entry := event.InspectEntry{
			Index: i,
			Kind:  act.Kind(),
			Desc:  act.Desc(),
		}
		for _, op := range act.Ops() {
			if insp, ok := op.(spec.OpInspector); ok {
				entry.Fields = append(entry.Fields, insp.Inspect()...)
			}
		}
		detail.Entries = append(detail.Entries, entry)
	}
	return detail, nil
}

// InspectDiffFile returns desired vs current content for a specific file op.
func (e *Engine) InspectDiffFile(ctx context.Context, destPath string) (*InspectDiffResult, error) {
	p, _, _, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return nil, err
	}
	e.storeSourcePaths(ctx, p)

	var found []diffableOp
	for _, act := range p.Deploy.Actions {
		for _, op := range act.Ops() {
			d, ok := op.(spec.Diffable)
			if !ok {
				continue
			}
			if strings.Contains(d.DestPath(), destPath) {
				found = append(found, diffableOp{diff: d, src: e.src, tgt: e.tgt})
			}
		}
	}

	if len(found) == 0 {
		err := noDiffableOpsError{CfgPath: e.cfg.Path, Filter: destPath}
		emitEngineDiagnostic(e.em, e.cfg.Path, err)
		return nil, AbortError{Causes: []error{err}}
	}

	if len(found) > 1 {
		paths := make([]string, len(found))
		for i, f := range found {
			paths[i] = f.diff.DestPath()
		}
		err := multipleDiffableOpsError{CfgPath: e.cfg.Path, Count: len(found), Paths: paths}
		emitEngineDiagnostic(e.em, e.cfg.Path, err)
		return nil, AbortError{Causes: []error{err}}
	}

	dop := found[0]

	desired, err := dop.diff.DesiredContent(ctx, dop.src, dop.tgt)
	if err != nil {
		return nil, err
	}

	current, err := dop.diff.CurrentContent(ctx, dop.src, dop.tgt)
	if err != nil {
		if target.IsNotExist(err) {
			current = nil
		} else {
			return nil, err
		}
	}

	return &InspectDiffResult{
		DestPath: dop.diff.DestPath(),
		Current:  current,
		Desired:  desired,
	}, nil
}

type diffableOp struct {
	diff spec.Diffable
	src  source.Source
	tgt  target.Target
}

// Diagnostics
// -----------------------------------------------------------------------------

type noDiffableOpsError struct {
	CfgPath string
	Filter  string
}

func (e noDiffableOpsError) Error() string {
	return fmt.Sprintf("no diffable ops matching %q", e.Filter)
}

func (e noDiffableOpsError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeNoDiffableOps,
			Text: `no diffable ops matching "{{.Filter}}"`,
			Hint: "use scampi inspect <config> --diff to list available paths",
			Data: e,
		},
	}
}

type multipleDiffableOpsError struct {
	CfgPath string
	Count   int
	Paths   []string
}

func (e multipleDiffableOpsError) Error() string {
	return fmt.Sprintf("found %d diffable ops, expected exactly one", e.Count)
}

func (e multipleDiffableOpsError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeMultipleDiffableOps,
			Text: `found {{.Count}} diffable ops — narrow your filter`,
			Hint: "destinations:\n{{range .Paths}}  {{.}}\n{{end}}",
			Data: e,
		},
	}
}
