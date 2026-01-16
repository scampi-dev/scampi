package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
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
	e.em.Emit(diagnostic.EngineStarted())

	cfgPath, err := filepath.Abs(cfgPath)
	if err != nil {
		panic(fmt.Errorf("BUG: filepath.Abs() failed: %w", err))
	}

	cfg, err := LoadConfigWithSource(e.em, cfgPath, store, e.src)
	if err != nil {
		return err
	}

	plan, err := plan(cfg, e.em)
	if err != nil {
		return err
	}

	e.em.Emit(diagnostic.PlanProduced(plan))

	e.em.Emit(diagnostic.EngineFinished(diagnostic.RunSummary{}, time.Since(start), err))

	return err
}

func plan(cfg spec.Config, em diagnostic.Emitter) (spec.Plan, error) {
	start := time.Now()
	em.Emit(diagnostic.PlanStarted())

	var (
		plan        spec.Plan
		causes      []error
		diagResults []diagnosticResult
	)

	for i, unit := range cfg.Units {
		act, err := unit.Type.Plan(i, unit)
		if err != nil {
			dr := emitDiagnostics(
				em,
				event.Subject{
					Index: i,
					Name:  unit.Name,
					Kind:  unit.Type.Kind(),
				},
				err,
			)

			diagResults = append(diagResults, dr)
			causes = append(causes, err)
			continue
		}

		plan.Actions = append(plan.Actions, act)
		em.Emit(diagnostic.UnitPlanned(i, act.Name(), unit.Type.Kind()))
	}

	em.Emit(diagnostic.PlanFinished(
		len(plan.Actions),
		len(causes),
		time.Since(start),
	))

	for _, dr := range diagResults {
		if dr.ShouldAbort() {
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
