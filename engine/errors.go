package engine

import (
	"errors"
	"runtime"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/util"
)

type AbortError struct {
	Causes []error
}

func (AbortError) Error() string {
	return "execution aborted"
}

type diagnosticResult struct {
	Impacts []diagnostic.Impact
}

func (r *diagnosticResult) add(impact diagnostic.Impact) {
	r.Impacts = append(r.Impacts, impact)
}

func (r diagnosticResult) ShouldAbort() bool {
	for _, i := range r.Impacts {
		if i&diagnostic.ImpactAbort != 0 {
			return true
		}
	}
	return false
}

func panicIfNotAbortError(err error) error {
	var abort AbortError
	if errors.As(err, &abort) {
		return abort
	}
	// very cold codepath
	wrap := util.BUG("Engine failed with non-signal error: %w", err)
	if pc, file, line, ok := runtime.Caller(1); ok {
		_ = file
		_ = line
		details := runtime.FuncForPC(pc)
		wrap = util.BUG("%s failed with non-signal error: %w", details.Name(), err)
	}
	panic(wrap)
}

func emitDiagnostics(
	em diagnostic.Emitter,
	subject event.Subject,
	err error,
) (diagnosticResult, bool) {
	var res diagnosticResult

	if err == nil {
		return res, false
	}

	var dp diagnostic.DiagnosticProvider
	if !errors.As(err, &dp) {
		return res, false
	}

	for _, ev := range dp.Diagnostics(subject) {
		em.Emit(ev)

		// determine impact
		impact := diagnostic.ImpactAbort // safe default
		if ip, ok := err.(diagnostic.ImpactProvider); ok {
			impact = ip.Impact()
		}
		res.add(impact)
	}

	return res, true
}
