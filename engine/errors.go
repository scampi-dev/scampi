package engine

import (
	"errors"
	"fmt"
	"runtime"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type AbortError struct {
	Causes []error
}

func (AbortError) Error() string {
	return "execution aborted"
}

type CapabilityMismatch struct {
	StepIndex    int
	StepKind     string
	RequiredCaps capability.Capability
	MissingCaps  capability.Capability
	ProvidedCaps capability.Capability
	Source       spec.SourceSpan
}

func (e CapabilityMismatch) Error() string {
	return fmt.Sprintf(
		"step %q requires %s, but target only provides %s (missing: %s)",
		e.StepKind, e.RequiredCaps, e.ProvidedCaps, e.MissingCaps,
	)
}

func (e CapabilityMismatch) EventTemplate() event.Template {
	return event.Template{
		ID:   "engine.CapabilityMismatch",
		Text: `step "{{.StepKind}}" requires capabilities not provided by target`,
		Hint: "use a different target or remove incompatible steps",
		Help: fmt.Sprintf(
			"missing:  %s\nrequired: %s\nprovided: %s",
			e.MissingCaps, e.RequiredCaps, e.ProvidedCaps,
		),
		Data:   e,
		Source: &e.Source,
	}
}

func (e CapabilityMismatch) Severity() signal.Severity {
	return signal.Error
}

func (e CapabilityMismatch) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func panicIfNotAbortError(err error) error {
	var abort AbortError
	if errors.As(err, &abort) {
		return abort
	}
	// very cold codepath
	wrap := errs.BUG("Engine failed with non-signal error: %w", err)
	if pc, file, line, ok := runtime.Caller(1); ok {
		_ = file
		_ = line
		details := runtime.FuncForPC(pc)
		wrap = errs.BUG("%s failed with non-signal error: %w", details.Name(), err)
	}
	panic(wrap)
}

// emitEngineDiagnostic emits a diagnostic for engine-level errors.
// Returns the impact and whether a diagnostic was emitted.
func emitEngineDiagnostic(
	em diagnostic.Emitter,
	cfgPath string,
	err error,
) (diagnostic.Impact, bool) {
	if err == nil {
		return 0, false
	}

	var ds diagnostic.Diagnostics
	if errors.As(err, &ds) {
		impact := diagnostic.ImpactNone
		for _, d := range ds {
			em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic(cfgPath, d))
			if d.Impact() > impact {
				impact = d.Impact()
			}
		}

		return impact, true
	}

	var d diagnostic.Diagnostic
	if !errors.As(err, &d) {
		return 0, false
	}

	em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic(cfgPath, d))
	return d.Impact(), true
}

// emitPlanDiagnostic emits a diagnostic for plan-level errors.
// Returns the impact and whether a diagnostic was emitted.
func emitPlanDiagnostic(
	em diagnostic.Emitter,
	stepIndex int,
	stepKind string,
	stepDesc string,
	err error,
) (diagnostic.Impact, bool) {
	if err == nil {
		return 0, false
	}

	var ds diagnostic.Diagnostics
	if errors.As(err, &ds) {
		impact := diagnostic.ImpactNone
		for _, d := range ds {
			em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(
				stepIndex, stepKind, stepDesc, d,
			))
			if d.Impact() > impact {
				impact = d.Impact()
			}
		}

		return impact, true
	}

	var d diagnostic.Diagnostic
	if !errors.As(err, &d) {
		return 0, false
	}

	em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(
		stepIndex, stepKind, stepDesc, d,
	))
	return d.Impact(), true
}

// emitActionDiagnostic emits a diagnostic for action-level errors.
// Returns the impact and whether a diagnostic was emitted.
func emitActionDiagnostic(
	em diagnostic.Emitter,
	stepIndex int,
	stepKind string,
	stepDesc string,
	err error,
) (diagnostic.Impact, bool) {
	if err == nil {
		return 0, false
	}

	var ds diagnostic.Diagnostics
	if errors.As(err, &ds) {
		impact := diagnostic.ImpactNone
		for _, d := range ds {
			em.EmitActionDiagnostic(diagnostic.RaiseActionDiagnostic(
				stepIndex, stepKind, stepDesc, d,
			))
			if d.Impact() > impact {
				impact = d.Impact()
			}
		}

		return impact, true
	}

	var d diagnostic.Diagnostic
	if !errors.As(err, &d) {
		return 0, false
	}

	em.EmitActionDiagnostic(diagnostic.RaiseActionDiagnostic(
		stepIndex, stepKind, stepDesc, d,
	))
	return d.Impact(), true
}

// emitOpDiagnostic emits a diagnostic for op-level errors.
// Returns the impact and whether a diagnostic was emitted.
func emitOpDiagnostic(
	em diagnostic.Emitter,
	stepIndex int,
	stepKind string,
	stepDesc string,
	displayID string,
	err error,
) (diagnostic.Impact, bool) {
	if err == nil {
		return 0, false
	}

	var ds diagnostic.Diagnostics
	if errors.As(err, &ds) {
		impact := diagnostic.ImpactNone
		for _, d := range ds {
			em.EmitOpDiagnostic(diagnostic.RaiseOpDiagnostic(
				stepIndex, stepKind, stepDesc, displayID, d,
			))
			if d.Impact() > impact {
				impact = d.Impact()
			}
		}

		return impact, true
	}

	var d diagnostic.Diagnostic
	if !errors.As(err, &d) {
		return 0, false
	}

	em.EmitOpDiagnostic(diagnostic.RaiseOpDiagnostic(
		stepIndex, stepKind, stepDesc, displayID, d,
	))
	return d.Impact(), true
}
