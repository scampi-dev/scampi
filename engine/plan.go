package engine

import (
	"context"
	"path/filepath"
	"time"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/model"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
)

func Plan(ctx context.Context, em diagnostic.Emitter, cfgPath string, store *spec.SourceStore) error {
	e := New(
		source.LocalPosixSource{},
		nil, // Plan must NEVER touch target, not even reads
		em,
	)

	return e.Plan(ctx, cfgPath, store)
}

func (e *Engine) Plan(ctx context.Context, cfgPath string, store *spec.SourceStore) error {
	start := time.Now()
	e.em.EmitEngineLifecycle(diagnostic.EngineStarted())

	cfgPath, err := filepath.Abs(cfgPath)
	if err != nil {
		panic(errs.BUG("filepath.Abs() failed: %w", err))
	}

	cfg, err := LoadConfigWithSource(ctx, e.em, cfgPath, store, e.src)
	if err != nil {
		return err
	}

	plan, err := plan(cfg, e.em)
	if err != nil {
		return err
	}

	e.em.EmitPlanLifecycle(diagnostic.PlanProduced(plan))

	e.em.EmitEngineLifecycle(diagnostic.EngineFinished(model.ExecutionReport{}, time.Since(start), err, false))

	return err
}

func plan(cfg spec.Config, em diagnostic.Emitter) (spec.Plan, error) {
	start := time.Now()
	em.EmitPlanLifecycle(diagnostic.PlanStarted(cfg.Unit.ID))

	var (
		causes  []error
		impacts []diagnostic.Impact
	)

	plan := spec.Plan{
		Unit: spec.Unit{
			ID:   cfg.Unit.ID,
			Desc: cfg.Unit.Desc,
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
		if impact.Is(diagnostic.ImpactAbort) {
			return spec.Plan{}, AbortError{
				Causes: causes,
			}
		}
	}

	if err := DetectPlanCycles(em, plan); err != nil {
		return spec.Plan{}, err
	}

	return plan, nil
}
