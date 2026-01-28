package engine

import (
	"errors"
	"runtime"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/errs"
)

type AbortError struct {
	Causes []error
}

func (AbortError) Error() string {
	return "execution aborted"
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

	var d diagnostic.Diagnostic
	if !errors.As(err, &d) {
		return 0, false
	}

	em.EmitOpDiagnostic(diagnostic.RaiseOpDiagnostic(
		stepIndex, stepKind, stepDesc, displayID, d,
	))
	return d.Impact(), true
}
