// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"sync"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

// Plan loads cfgPath, resolves it into one plan per deploy block,
// and returns the cross-deploy schedule with each deploy's step
// tree attached as a leaf. The same shape covers the single-deploy
// case (one level, one node, no edges).
func Plan(
	ctx diagnostic.Ctx,
	cfgPath string,
	store *diagnostic.SourceStore,
	opts spec.ResolveOptions,
) (result.Plan, error) {
	src := source.WithRoot(cfgPath, source.LocalPosixSource{})
	cfg, err := LoadConfig(ctx, cfgPath, store, src)
	if err != nil {
		return result.Plan{}, err
	}

	resolved, err := ResolveMultiple(cfg, opts)
	if err != nil {
		if impact, ok := emitEngineDiagnostic(ctx, cfgPath, err); ok {
			if impact.ShouldAbort() {
				return result.Plan{}, AbortError{Causes: []error{err}}
			}
		}
		return result.Plan{}, err
	}

	// Build the cross-deploy graph. For a single deploy this is a
	// trivial one-level, one-node graph with no edges - same shape,
	// no special case.
	graph, err := buildDeployGraph(resolved)
	if err != nil {
		return result.Plan{}, err
	}

	// Plan each deploy concurrently and key the resulting PlanDetail
	// by the resolved config's position in the input slice so we can
	// stitch it back onto the matching graph node when assembling
	// the levels structure.
	allCaps := capabilityTarget{caps: capability.All}
	var (
		mu      sync.Mutex
		details = make(map[int]result.PlanDetail, len(resolved))
	)
	err = runPlansConcurrent(ctx, resolved, func(ctx diagnostic.Ctx, res spec.Config) error {
		e, eErr := NewWithTarget(ctx, src, res, allCaps)
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
		return result.Plan{}, err
	}

	levels := make([]result.DeployLevel, len(graph.levels))
	for li, level := range graph.levels {
		nodes := make([]result.DeployPlan, len(level))
		for ni, n := range level {
			nodes[ni] = result.DeployPlan{
				DeployName: n.res.DeployName,
				TargetName: n.res.TargetName,
				After:      depNames(n),
				Needs:      driverResources(n),
				Detail:     details[n.idx],
			}
		}
		// Stable within-level ordering by deploy name keeps output
		// deterministic across runs.
		slices.SortStableFunc(nodes, func(a, b result.DeployPlan) int {
			return strings.Compare(a.DeployName, b.DeployName)
		})
		levels[li] = result.DeployLevel{Index: li, Nodes: nodes}
	}
	return result.Plan{Levels: levels}, nil
}

// resolvedIndex returns the position of res in resolved by
// deploy+target name (which together uniquely identify a resolved
// config within a single run). Returns -1 if not found, which should
// never happen because we only key configs from the same slice we
// iterate.
func resolvedIndex(resolved []spec.Config, res spec.Config) int {
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
func (e *Engine) PlanDeploy(ctx diagnostic.Ctx) (result.PlanDetail, error) {
	p, stepDeps, _, err := plan(e.cfg, ctx, e.tgt.Capabilities())
	if err != nil {
		return result.PlanDetail{}, err
	}
	e.storeSourcePaths(ctx, p)
	return planDetail(p, stepDeps), nil
}

// planDetail assembles the rendering payload for one resolved plan.
// Mirrors the previous diagnostic.PlanProduced factory; lives here
// now that plan output is a return value, not an event.
func planDetail(p spec.Plan, stepDeps StepDeps) result.PlanDetail {
	var allOps []spec.Op
	opIndex := make(map[spec.Op]int)
	stepOpBase := make(map[int]int) // step index -> first op index
	for i, act := range p.Deploy.Steps {
		stepOpBase[i] = len(allOps)
		for _, op := range act.Ops() {
			opIndex[op] = len(allOps)
			allOps = append(allOps, op)
		}
	}

	plannedOps := make([]result.PlannedOp, len(allOps))
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
		plannedOps[i] = result.PlannedOp{
			Index:     i,
			DisplayID: diagnostic.OpDisplayID(op),
			DependsOn: deps,
			Template:  tmpl,
		}
	}

	detail := result.PlanDetail{DeployID: string(p.Deploy.ID), DeployDesc: p.Deploy.Desc}
	for i, act := range p.Deploy.Steps {
		start := stepOpBase[i]
		end := start + len(act.Ops())
		var deps []int
		if stepDeps != nil && i < len(stepDeps) {
			deps = stepDeps[i]
		}
		detail.Steps = append(detail.Steps, result.PlannedStep{
			Index:     i,
			Desc:      act.Desc(),
			Kind:      act.Kind(),
			DependsOn: deps,
			Ops:       plannedOps[start:end],
		})
	}
	return detail
}

// StepDeps maps step index to indices of steps it depends on.
type StepDeps [][]int

// hookPlan holds planned hook steps and the mapping from step index
// to the hook IDs it should notify on change.
type hookPlan struct {
	steps    map[string][]spec.Step // hook ID → planned steps
	onChange map[int][]string       // step index → hook IDs
}

func plan(
	cfg spec.Config,
	ctx diagnostic.Ctx,
	tgtCaps capability.Capability,
) (spec.Plan, StepDeps, *hookPlan, error) {
	deployID := spec.DeployID(cfg.DeployName)

	steps, stepSources, onChange, causes, impacts := planSteps(cfg.Steps, ctx, tgtCaps)
	hookSteps, hookCauses, hookImpacts := planHooks(cfg.Hooks, ctx, tgtCaps)
	causes = append(causes, hookCauses...)
	impacts = append(impacts, hookImpacts...)

	p := spec.Plan{
		Deploy: spec.Deploy{
			ID:    deployID,
			Desc:  cfg.DeployName,
			Steps: steps,
		},
	}
	hp := &hookPlan{steps: hookSteps, onChange: onChange}

	for _, impact := range impacts {
		if impact.ShouldAbort() {
			return spec.Plan{}, nil, nil, AbortError{Causes: causes}
		}
	}

	if err := validateHooks(ctx, cfg, hp); err != nil {
		return spec.Plan{}, nil, nil, err
	}
	if err := detectDuplicatePromises(ctx, steps, stepSources, cfg.Steps); err != nil {
		return spec.Plan{}, nil, nil, err
	}
	if err := DetectPlanCycles(ctx, p); err != nil {
		return spec.Plan{}, nil, nil, err
	}

	nodes := buildStepGraph(p.Deploy.Steps)
	if err := DetectStepCycles(ctx, nodes); err != nil {
		return spec.Plan{}, nil, nil, err
	}

	return p, extractStepDeps(nodes), hp, nil
}

func planOneStep(
	step spec.DeclaredStep,
	stepIdx int,
	tgtCaps capability.Capability,
) (spec.Step, error) {
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
	declared []spec.DeclaredStep,
	ctx diagnostic.Ctx,
	tgtCaps capability.Capability,
) ([]spec.Step, []int, map[int][]string, []error, []diagnostic.Impact) {
	var steps []spec.Step
	var stepSources []int // stepSources[k] = source step index of steps[k]
	onChange := make(map[int][]string)
	var causes []error
	var impacts []diagnostic.Impact

	for i, decl := range declared {
		planned, err := planOneStep(decl, i, tgtCaps)
		if err != nil {
			impact, _ := emitPlanDiagnostic(ctx, i, decl.Type.Kind(), decl.Desc, err)
			impacts = append(impacts, impact)
			if planned == nil || impact.ShouldAbort() {
				causes = append(causes, err)
				continue
			}
		}

		stepIdx := len(steps)
		steps = append(steps, planned)
		stepSources = append(stepSources, i)

		if len(decl.OnChange) > 0 {
			onChange[stepIdx] = decl.OnChange
		}
	}

	return steps, stepSources, onChange, causes, impacts
}

func planHooks(
	hooks map[string][]spec.DeclaredStep,
	ctx diagnostic.Ctx,
	tgtCaps capability.Capability,
) (map[string][]spec.Step, []error, []diagnostic.Impact) {
	planned := make(map[string][]spec.Step)
	var causes []error
	var impacts []diagnostic.Impact

	for id, declared := range hooks {
		var hookSteps []spec.Step
		hookFailed := false
		for _, decl := range declared {
			step, err := planOneStep(decl, -1, tgtCaps)
			if err != nil {
				impact, _ := emitPlanDiagnostic(ctx, -1, decl.Type.Kind(), decl.Desc, err)
				impacts = append(impacts, impact)
				causes = append(causes, err)
				hookFailed = true
				break
			}
			hookSteps = append(hookSteps, step)
		}
		if !hookFailed {
			planned[id] = hookSteps
		}
	}

	return planned, causes, impacts
}

// validateHooks checks that all on_change references point to defined hooks
// and that hook chains don't form cycles.
func validateHooks(ctx diagnostic.Ctx, cfg spec.Config, hp *hookPlan) error {
	// Check step on_change references
	for i, step := range cfg.Steps {
		for _, hookID := range step.OnChange {
			if _, ok := hp.steps[hookID]; !ok {
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
				impact, _ := emitPlanDiagnostic(ctx, i, step.Type.Kind(), step.Desc, err)
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
				if _, ok := hp.steps[hookID]; !ok {
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
					impact, _ := emitPlanDiagnostic(ctx, -1, step.Type.Kind(), "hook:"+id, err)
					if impact.ShouldAbort() {
						return AbortError{Causes: []error{err}}
					}
				}
			}
		}
	}

	// Detect cycles in hook chains
	if err := detectHookCycles(ctx, cfg.Hooks); err != nil {
		return err
	}

	return nil
}

// extractStepDeps converts step graph nodes to dependency indices.
func extractStepDeps(nodes []*stepNode) StepDeps {
	deps := make(StepDeps, len(nodes))
	for _, n := range nodes {
		for _, req := range n.requires {
			deps[n.idx] = append(deps[n.idx], req.idx)
		}
	}
	return deps
}

func collectRequiredCaps(act spec.Step) capability.Capability {
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
