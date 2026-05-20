// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"sync"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// Plan loads cfgPath, resolves it into one plan per deploy block,
// and returns the cross-deploy schedule with each deploy's action
// tree attached as a leaf. The same shape covers the single-deploy
// case (one level, one node, no edges).
func Plan(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) (PlanResult, error) {
	src := source.WithRoot(cfgPath, source.LocalPosixSource{})
	cfg, err := LoadConfig(ctx, em, cfgPath, store, src)
	if err != nil {
		return PlanResult{}, err
	}

	resolved, err := ResolveMultiple(cfg, opts)
	if err != nil {
		if impact, ok := emitEngineDiagnostic(em, cfgPath, err); ok {
			if impact.ShouldAbort() {
				return PlanResult{}, AbortError{Causes: []error{err}}
			}
		}
		return PlanResult{}, err
	}

	// Build the cross-deploy graph. For a single deploy this is a
	// trivial one-level, one-node graph with no edges - same shape,
	// no special case.
	graph, err := buildDeployGraph(resolved)
	if err != nil {
		return PlanResult{}, err
	}

	// Plan each deploy concurrently and key the resulting PlanDetail
	// by the resolved config's position in the input slice so we can
	// stitch it back onto the matching graph node when assembling
	// the levels structure.
	allCaps := capabilityTarget{caps: capability.All}
	var (
		mu      sync.Mutex
		details = make(map[int]event.PlanDetail, len(resolved))
	)
	err = runPlansConcurrent(ctx, em, resolved, func(ctx context.Context, res spec.ResolvedConfig) error {
		e, eErr := NewWithTarget(ctx, src, res, em, allCaps)
		if eErr != nil {
			return eErr
		}
		defer e.Close()
		e.store = store

		detail, dErr := e.PlanDeploy(ctx)
		if dErr != nil {
			return dErr
		}
		idx := resolvedIndex(resolved, res)
		mu.Lock()
		details[idx] = detail
		mu.Unlock()
		return nil
	})
	if err != nil {
		return PlanResult{}, err
	}

	levels := make([]DeployLevel, len(graph.levels))
	for li, level := range graph.levels {
		nodes := make([]DeployPlan, len(level))
		for ni, n := range level {
			nodes[ni] = DeployPlan{
				DeployName: n.res.DeployName,
				TargetName: n.res.TargetName,
				After:      depNames(n),
				Needs:      driverResources(n),
				Detail:     details[n.idx],
			}
		}
		// Stable within-level ordering by deploy name keeps output
		// deterministic across runs.
		slices.SortStableFunc(nodes, func(a, b DeployPlan) int {
			return strings.Compare(a.DeployName, b.DeployName)
		})
		levels[li] = DeployLevel{Index: li, Nodes: nodes}
	}
	return PlanResult{Levels: levels}, nil
}

// resolvedIndex returns the position of res in resolved by
// deploy+target name (which together uniquely identify a resolved
// config within a single run). Returns -1 if not found, which should
// never happen because we only key configs from the same slice we
// iterate.
func resolvedIndex(resolved []spec.ResolvedConfig, res spec.ResolvedConfig) int {
	for i, r := range resolved {
		if r.DeployName == res.DeployName && r.TargetName == res.TargetName {
			return i
		}
	}
	return -1
}

// PlanDeploy builds the renderable plan detail for this engine's
// resolved deploy. The top-level engine.Plan aggregates per-deploy
// details into a PlanResult; tests can call this directly to
// surface plan-time errors without exercising the full pipeline.
func (e *Engine) PlanDeploy(ctx context.Context) (event.PlanDetail, error) {
	p, actionDeps, _, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return event.PlanDetail{}, err
	}
	e.storeSourcePaths(ctx, p)
	return planDetail(p, actionDeps), nil
}

// planDetail assembles the rendering payload for one resolved plan.
// Mirrors the previous diagnostic.PlanProduced factory; lives here
// now that plan output is a return value, not an event.
func planDetail(p spec.Plan, actionDeps ActionDeps) event.PlanDetail {
	var allOps []spec.Op
	opIndex := make(map[spec.Op]int)
	actionOpBase := make(map[int]int) // action index -> first op index
	for i, act := range p.Unit.Actions {
		actionOpBase[i] = len(allOps)
		for _, op := range act.Ops() {
			opIndex[op] = len(allOps)
			allOps = append(allOps, op)
		}
	}

	plannedOps := make([]event.PlannedOp, len(allOps))
	for i, op := range allOps {
		var tmpl *spec.PlanTemplate
		if d, ok := op.(spec.OpDescriber); ok {
			if desc := d.OpDescription(); desc != nil {
				t := desc.PlanTemplate()
				tmpl = &t
			}
		}
		var deps []int
		for _, dep := range op.DependsOn() {
			deps = append(deps, opIndex[dep])
		}
		plannedOps[i] = event.PlannedOp{
			Index:     i,
			DisplayID: diagnostic.OpDisplayID(op),
			DependsOn: deps,
			Template:  tmpl,
		}
	}

	detail := event.PlanDetail{UnitID: string(p.Unit.ID), UnitDesc: p.Unit.Desc}
	for i, act := range p.Unit.Actions {
		start := actionOpBase[i]
		end := start + len(act.Ops())
		var deps []int
		if actionDeps != nil && i < len(actionDeps) {
			deps = actionDeps[i]
		}
		detail.Actions = append(detail.Actions, event.PlannedAction{
			Index:     i,
			Desc:      act.Desc(),
			Kind:      act.Kind(),
			DependsOn: deps,
			Ops:       plannedOps[start:end],
		})
	}
	return detail
}

// ActionDeps maps action index to indices of actions it depends on.
type ActionDeps [][]int

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
) (spec.Plan, ActionDeps, *hookPlan, error) {
	unitID := spec.UnitID(cfg.DeployName)

	actions, actionSteps, onChange, causes, impacts := planSteps(cfg.Steps, em, tgtCaps)
	hookActions, hookCauses, hookImpacts := planHooks(cfg.Hooks, em, tgtCaps)
	causes = append(causes, hookCauses...)
	impacts = append(impacts, hookImpacts...)

	p := spec.Plan{
		Unit: spec.Unit{
			ID:      unitID,
			Desc:    cfg.DeployName,
			Actions: actions,
		},
	}
	hp := &hookPlan{actions: hookActions, onChange: onChange}

	for _, impact := range impacts {
		if impact.ShouldAbort() {
			return spec.Plan{}, nil, nil, AbortError{Causes: causes}
		}
	}

	if err := validateHooks(em, cfg, hp); err != nil {
		return spec.Plan{}, nil, nil, err
	}
	if err := detectDuplicatePromises(em, actions, actionSteps, cfg.Steps); err != nil {
		return spec.Plan{}, nil, nil, err
	}
	if err := DetectPlanCycles(em, p); err != nil {
		return spec.Plan{}, nil, nil, err
	}

	nodes := buildActionGraph(p.Unit.Actions)
	if err := DetectActionCycles(em, nodes); err != nil {
		return spec.Plan{}, nil, nil, err
	}

	return p, extractActionDeps(nodes), hp, nil
}

func planOneStep(
	step spec.StepInstance,
	stepIdx int,
	tgtCaps capability.Capability,
) (spec.Action, error) {
	act, err := step.Type.Plan(step)
	if err != nil && act == nil {
		return nil, err
	}
	reqCaps := collectRequiredCaps(act)
	if misCaps := reqCaps &^ tgtCaps; misCaps != 0 {
		return nil, CapabilityMismatchError{
			StepIndex:    stepIdx,
			StepKind:     act.Kind(),
			RequiredCaps: reqCaps,
			MissingCaps:  misCaps,
			ProvidedCaps: tgtCaps,
			Source:       step.Source,
		}
	}
	return act, err
}

func planSteps(
	steps []spec.StepInstance,
	em diagnostic.Emitter,
	tgtCaps capability.Capability,
) ([]spec.Action, []int, map[int][]string, []error, []diagnostic.Impact) {
	var actions []spec.Action
	var actionSteps []int // actionSteps[k] = source step index of actions[k]
	onChange := make(map[int][]string)
	var causes []error
	var impacts []diagnostic.Impact

	for i, step := range steps {
		act, err := planOneStep(step, i, tgtCaps)
		if err != nil {
			impact, _ := emitPlanDiagnostic(em, i, step.Type.Kind(), step.Desc, err)
			impacts = append(impacts, impact)
			if act == nil || impact.ShouldAbort() {
				causes = append(causes, err)
				continue
			}
		}

		actionIdx := len(actions)
		actions = append(actions, act)
		actionSteps = append(actionSteps, i)

		if len(step.OnChange) > 0 {
			onChange[actionIdx] = step.OnChange
		}
	}

	return actions, actionSteps, onChange, causes, impacts
}

func planHooks(
	hooks map[string][]spec.StepInstance,
	em diagnostic.Emitter,
	tgtCaps capability.Capability,
) (map[string][]spec.Action, []error, []diagnostic.Impact) {
	actions := make(map[string][]spec.Action)
	var causes []error
	var impacts []diagnostic.Impact

	for id, steps := range hooks {
		var hookActions []spec.Action
		hookFailed := false
		for _, step := range steps {
			act, err := planOneStep(step, -1, tgtCaps)
			if err != nil {
				impact, _ := emitPlanDiagnostic(em, -1, step.Type.Kind(), step.Desc, err)
				impacts = append(impacts, impact)
				causes = append(causes, err)
				hookFailed = true
				break
			}
			hookActions = append(hookActions, act)
		}
		if !hookFailed {
			actions[id] = hookActions
		}
	}

	return actions, causes, impacts
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
func extractActionDeps(nodes []*actionNode) ActionDeps {
	deps := make(ActionDeps, len(nodes))
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

func (capabilityTarget) ReadDir(_ context.Context, _ string) ([]fs.DirEntry, error) {
	panic(errs.BUG("ReadDir called on capability-only target"))
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

func (capabilityTarget) InspectContainer(_ context.Context, _ string) (target.ContainerInfo, bool, error) {
	panic(errs.BUG("InspectContainer called on capability-only target"))
}

func (capabilityTarget) CreateContainer(_ context.Context, _ target.ContainerInfo) error {
	panic(errs.BUG("CreateContainer called on capability-only target"))
}

func (capabilityTarget) StartContainer(_ context.Context, _ string) error {
	panic(errs.BUG("StartContainer called on capability-only target"))
}

func (capabilityTarget) StopContainer(_ context.Context, _ string) error {
	panic(errs.BUG("StopContainer called on capability-only target"))
}

func (capabilityTarget) RemoveContainer(_ context.Context, _ string) error {
	panic(errs.BUG("RemoveContainer called on capability-only target"))
}
