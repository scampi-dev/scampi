package engine

import (
	"context"
	"io/fs"
	"time"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/model"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

func Plan(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
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

func (e *Engine) Plan(_ context.Context) error {
	start := time.Now()
	e.em.EmitEngineLifecycle(diagnostic.EngineStarted())

	plan, actionDeps, err := plan(e.cfg, e.em, e.tgt.Capabilities())
	if err != nil {
		return err
	}

	e.em.EmitPlanLifecycle(diagnostic.PlanProduced(plan, actionDeps))

	e.em.EmitEngineLifecycle(diagnostic.EngineFinished(model.ExecutionReport{}, time.Since(start), err, false))

	return err
}

func plan(
	cfg spec.ResolvedConfig, em diagnostic.Emitter, tgtCaps capability.Capability,
) (spec.Plan, diagnostic.ActionDeps, error) {
	start := time.Now()
	unitID := spec.UnitID(cfg.DeployName)
	em.EmitPlanLifecycle(diagnostic.PlanStarted(unitID))

	var (
		causes  []error
		impacts []diagnostic.Impact
	)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   unitID,
			Desc: cfg.DeployName,
		},
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

		plan.Unit.Actions = append(plan.Unit.Actions, act)
		em.EmitPlanLifecycle(diagnostic.StepPlanned(i, act.Desc(), step.Type.Kind()))
	}

	em.EmitPlanLifecycle(diagnostic.PlanFinished(
		plan.Unit.ID,
		len(plan.Unit.Actions),
		len(causes),
		time.Since(start),
	))

	for _, impact := range impacts {
		if impact.ShouldAbort() {
			return spec.Plan{}, nil, AbortError{
				Causes: causes,
			}
		}
	}

	if err := DetectPlanCycles(em, plan); err != nil {
		return spec.Plan{}, nil, err
	}

	nodes := buildActionGraph(plan.Unit.Actions)
	if err := DetectActionCycles(em, nodes); err != nil {
		return spec.Plan{}, nil, err
	}

	actionDeps := extractActionDeps(nodes)

	return plan, actionDeps, nil
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
