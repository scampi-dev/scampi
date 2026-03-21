// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"io/fs"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/model"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

func Plan(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) error {
	return runForEachResolved(ctx, em, cfgPath, store, opts, func(ctx context.Context, e *Engine) error {
		// Plan must NEVER touch target, not even reads
		e.tgt = capabilityTarget{
			caps: capability.All,
		}
		return e.Plan(ctx)
	})
}

func (e *Engine) Plan(ctx context.Context) error {
	start := time.Now()
	e.em.EmitEngineLifecycle(diagnostic.EngineStarted())

	plan, actionDeps, _, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return err
	}
	e.storeSourcePaths(ctx, plan)

	e.em.EmitPlanLifecycle(diagnostic.PlanProduced(plan, actionDeps))

	e.em.EmitEngineLifecycle(diagnostic.EngineFinished(model.ExecutionReport{}, 0, time.Since(start), err, false))

	return err
}

// hookPlan holds planned hook actions and the mapping from action index
// to the hook IDs it should notify on change.
type hookPlan struct {
	actions  map[string][]spec.Action // hook ID → planned actions
	onChange map[int][]string         // step action index → hook IDs
}

func plan(
	cfg spec.ResolvedConfig,
	em diagnostic.Emitter,
	tgtCaps capability.Capability,
) (spec.Plan, diagnostic.ActionDeps, *hookPlan, error) {
	start := time.Now()
	unitID := spec.UnitID(cfg.DeployName)
	em.EmitPlanLifecycle(diagnostic.PlanStarted(unitID))

	var (
		causes  []error
		impacts []diagnostic.Impact
	)

	p := spec.Plan{
		Unit: spec.Unit{
			ID:   unitID,
			Desc: cfg.DeployName,
		},
	}

	hp := &hookPlan{
		actions:  make(map[string][]spec.Action),
		onChange: make(map[int][]string),
	}

	for i, step := range cfg.Steps {
		act, err := step.Type.Plan(i, step)
		if err != nil {
			impact, _ := emitPlanDiagnostic(em, i, step.Type.Kind(), step.Desc, err)
			impacts = append(impacts, impact)
			causes = append(causes, err)
			continue
		}

		reqCaps := collectRequiredCaps(act)
		if misCaps := reqCaps &^ tgtCaps; misCaps != 0 {
			err := CapabilityMismatchError{
				StepIndex:    i,
				StepKind:     act.Kind(),
				RequiredCaps: reqCaps,
				MissingCaps:  misCaps,
				ProvidedCaps: tgtCaps,
				Source:       step.Source,
			}
			impact, _ := emitPlanDiagnostic(em, i, step.Type.Kind(), step.Desc, err)
			impacts = append(impacts, impact)
			causes = append(causes, err)
			continue
		}

		actionIdx := len(p.Unit.Actions)
		p.Unit.Actions = append(p.Unit.Actions, act)
		em.EmitPlanLifecycle(diagnostic.StepPlanned(i, act.Desc(), step.Type.Kind()))

		if len(step.OnChange) > 0 {
			hp.onChange[actionIdx] = step.OnChange
		}
	}

	// Plan hooks
	for id, steps := range cfg.Hooks {
		var hookActions []spec.Action
		hookFailed := false
		for _, step := range steps {
			act, err := step.Type.Plan(-1, step)
			if err != nil {
				impact, _ := emitPlanDiagnostic(em, -1, step.Type.Kind(), step.Desc, err)
				impacts = append(impacts, impact)
				causes = append(causes, err)
				hookFailed = true
				break
			}

			reqCaps := collectRequiredCaps(act)
			if misCaps := reqCaps &^ tgtCaps; misCaps != 0 {
				err := CapabilityMismatchError{
					StepIndex:    -1,
					StepKind:     act.Kind(),
					RequiredCaps: reqCaps,
					MissingCaps:  misCaps,
					ProvidedCaps: tgtCaps,
					Source:       step.Source,
				}
				impact, _ := emitPlanDiagnostic(em, -1, step.Type.Kind(), step.Desc, err)
				impacts = append(impacts, impact)
				causes = append(causes, err)
				hookFailed = true
				break
			}

			hookActions = append(hookActions, act)
		}
		if !hookFailed {
			hp.actions[id] = hookActions
		}
	}

	em.EmitPlanLifecycle(diagnostic.PlanFinished(
		p.Unit.ID,
		len(p.Unit.Actions),
		len(causes),
		time.Since(start),
	))

	for _, impact := range impacts {
		if impact.ShouldAbort() {
			return spec.Plan{}, nil, nil, AbortError{
				Causes: causes,
			}
		}
	}

	// Validate on_change references and detect hook cycles
	if err := validateHooks(em, cfg, hp); err != nil {
		return spec.Plan{}, nil, nil, err
	}

	if err := DetectPlanCycles(em, p); err != nil {
		return spec.Plan{}, nil, nil, err
	}

	nodes := buildActionGraph(p.Unit.Actions)
	if err := DetectActionCycles(em, nodes); err != nil {
		return spec.Plan{}, nil, nil, err
	}

	actionDeps := extractActionDeps(nodes)

	return p, actionDeps, hp, nil
}

// validateHooks checks that all on_change references point to defined hooks
// and that hook chains don't form cycles.
func validateHooks(em diagnostic.Emitter, cfg spec.ResolvedConfig, hp *hookPlan) error {
	// Check step on_change references
	for i, step := range cfg.Steps {
		for _, hookID := range step.OnChange {
			if _, ok := hp.actions[hookID]; !ok {
				source := step.Source
				if fs, ok := step.Fields["on_change"]; ok {
					source = fs.Value
				}
				err := UnknownHookError{
					HookID:   hookID,
					StepKind: step.Type.Kind(),
					StepDesc: step.Desc,
					Source:   source,
				}
				impact, _ := emitPlanDiagnostic(em, i, step.Type.Kind(), step.Desc, err)
				if impact.ShouldAbort() {
					return AbortError{Causes: []error{err}}
				}
			}
		}
	}

	// Check hook on_change references (chaining)
	for id, steps := range cfg.Hooks {
		for _, step := range steps {
			for _, hookID := range step.OnChange {
				if _, ok := hp.actions[hookID]; !ok {
					source := step.Source
					if fs, ok := step.Fields["on_change"]; ok {
						source = fs.Value
					}
					err := UnknownHookError{
						HookID:   hookID,
						StepKind: step.Type.Kind(),
						StepDesc: step.Desc,
						Source:   source,
					}
					impact, _ := emitPlanDiagnostic(em, -1, step.Type.Kind(), "hook:"+id, err)
					if impact.ShouldAbort() {
						return AbortError{Causes: []error{err}}
					}
				}
			}
		}
	}

	// Detect cycles in hook chains
	if err := detectHookCycles(em, cfg.Hooks); err != nil {
		return err
	}

	return nil
}

// extractActionDeps converts action graph nodes to dependency indices.
func extractActionDeps(nodes []*actionNode) diagnostic.ActionDeps {
	deps := make(diagnostic.ActionDeps, len(nodes))
	for _, n := range nodes {
		for _, req := range n.requires {
			deps[n.idx] = append(deps[n.idx], req.idx)
		}
	}
	return deps
}

func collectRequiredCaps(act spec.Action) capability.Capability {
	var caps capability.Capability
	for _, op := range act.Ops() {
		caps |= op.RequiredCapabilities()
	}
	return caps
}

type capabilityTarget struct {
	caps capability.Capability
}

func (t capabilityTarget) Capabilities() capability.Capability { return t.caps }

func (capabilityTarget) ReadFile(_ context.Context, _ string) ([]byte, error) {
	panic(errs.BUG("ReadFile called on capability-only target"))
}

func (capabilityTarget) WriteFile(_ context.Context, _ string, _ []byte) error {
	panic(errs.BUG("WriteFile called on capability-only target"))
}

func (capabilityTarget) Stat(_ context.Context, _ string) (fs.FileInfo, error) {
	panic(errs.BUG("Stat called on capability-only target"))
}

func (capabilityTarget) Lstat(_ context.Context, _ string) (fs.FileInfo, error) {
	panic(errs.BUG("Lstat called on capability-only target"))
}

func (capabilityTarget) Readlink(_ context.Context, _ string) (string, error) {
	panic(errs.BUG("Readlink called on capability-only target"))
}

func (capabilityTarget) Symlink(_ context.Context, _, _ string) error {
	panic(errs.BUG("Symlink called on capability-only target"))
}

func (capabilityTarget) Remove(_ context.Context, _ string) error {
	panic(errs.BUG("Remove called on capability-only target"))
}

func (capabilityTarget) Chown(_ context.Context, _ string, _ target.Owner) error {
	panic(errs.BUG("Chown called on capability-only target"))
}

func (capabilityTarget) Chmod(_ context.Context, _ string, _ fs.FileMode) error {
	panic(errs.BUG("Chmod called on capability-only target"))
}

func (capabilityTarget) HasUser(_ context.Context, _ string) bool {
	panic(errs.BUG("HasUser called on capability-only target"))
}

func (capabilityTarget) HasGroup(_ context.Context, _ string) bool {
	panic(errs.BUG("HasGroup called on capability-only target"))
}

func (capabilityTarget) GetOwner(_ context.Context, _ string) (target.Owner, error) {
	panic(errs.BUG("GetOwner called on capability-only target"))
}

func (capabilityTarget) IsInstalled(_ context.Context, _ string) (bool, error) {
	panic(errs.BUG("IsInstalled called on capability-only target"))
}

func (capabilityTarget) InstallPkgs(_ context.Context, _ []string) error {
	panic(errs.BUG("InstallPkgs called on capability-only target"))
}

func (capabilityTarget) RemovePkgs(_ context.Context, _ []string) error {
	panic(errs.BUG("RemovePkgs called on capability-only target"))
}
