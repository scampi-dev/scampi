// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"fmt"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// InspectResult holds the content pair extracted by Inspect.
type InspectResult struct {
	DestPath string
	Current  []byte // nil when the file does not yet exist on target
	Desired  []byte
}

func Inspect(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
	stepFilter string,
) (*InspectResult, error) {
	var result *InspectResult

	err := forEachResolved(ctx, em, cfgPath, store, opts, func(ctx context.Context, e *Engine) error {
		r, err := e.Inspect(ctx, stepFilter)
		if err != nil {
			return err
		}
		result = r
		return nil
	})

	return result, err
}

func (e *Engine) Inspect(ctx context.Context, stepFilter string) (*InspectResult, error) {
	p, _, _, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return nil, err
	}
	e.storeSourcePaths(ctx, p)

	var found []inspectableOp
	for _, act := range p.Unit.Actions {
		for _, op := range act.Ops() {
			insp, ok := op.(spec.Inspectable)
			if !ok {
				continue
			}
			if stepFilter != "" && !strings.Contains(insp.DestPath(), stepFilter) {
				continue
			}
			found = append(found, inspectableOp{insp: insp, src: e.src, tgt: e.tgt})
		}
	}

	if len(found) == 0 {
		err := noInspectableOpsError{CfgPath: e.cfg.Path, Filter: stepFilter}
		emitEngineDiagnostic(e.em, e.cfg.Path, err)
		return nil, AbortError{Causes: []error{err}}
	}

	if len(found) > 1 {
		paths := make([]string, len(found))
		for i, f := range found {
			paths[i] = f.insp.DestPath()
		}
		err := multipleInspectableOpsError{CfgPath: e.cfg.Path, Count: len(found), Paths: paths}
		emitEngineDiagnostic(e.em, e.cfg.Path, err)
		return nil, AbortError{Causes: []error{err}}
	}

	iop := found[0]

	desired, err := iop.insp.DesiredContent(ctx, iop.src)
	if err != nil {
		return nil, err
	}

	current, err := iop.insp.CurrentContent(ctx, iop.src, iop.tgt)
	if err != nil {
		if target.IsNotExist(err) {
			current = nil
		} else {
			return nil, err
		}
	}

	return &InspectResult{
		DestPath: iop.insp.DestPath(),
		Current:  current,
		Desired:  desired,
	}, nil
}

type inspectableOp struct {
	insp spec.Inspectable
	src  source.Source
	tgt  target.Target
}

// Diagnostics
// -----------------------------------------------------------------------------

type noInspectableOpsError struct {
	diagnostic.FatalError
	CfgPath string
	Filter  string
}

func (e noInspectableOpsError) Error() string {
	if e.Filter != "" {
		return fmt.Sprintf("no inspectable ops matching %q", e.Filter)
	}
	return "no inspectable ops found"
}

func (e noInspectableOpsError) EventTemplate() event.Template {
	if e.Filter != "" {
		return event.Template{
			ID:   "engine.inspect.NoInspectableOps",
			Text: `no inspectable ops matching "{{.Filter}}"`,
			Hint: "check the --step filter value",
			Data: e,
		}
	}
	return event.Template{
		ID:   "engine.inspect.NoInspectableOps",
		Text: "no inspectable ops found in config",
		Hint: "inspect requires at least one file-content op (e.g. copy)",
	}
}

type multipleInspectableOpsError struct {
	diagnostic.FatalError
	CfgPath string
	Count   int
	Paths   []string
}

func (e multipleInspectableOpsError) Error() string {
	return fmt.Sprintf("found %d inspectable ops, expected exactly one", e.Count)
}

func (e multipleInspectableOpsError) EventTemplate() event.Template {
	return event.Template{
		ID:   "engine.inspect.MultipleInspectableOps",
		Text: `found {{.Count}} inspectable ops — use --step to pick one`,
		Hint: "destinations:\n{{range .Paths}}  {{.}}\n{{end}}",
		Data: e,
	}
}
