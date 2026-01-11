package engine

import (
	"errors"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
)

type AbortError struct {
	Causes []error
}

func (AbortError) Error() string {
	return "execution aborted"
}

type (
	diagnosticResult struct {
		Impacts []diagnostic.Impact
	}
)

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

func (r diagnosticResult) ShouldSkipUnit() bool {
	for _, i := range r.Impacts {
		if i&diagnostic.ImpactSkipUnit != 0 {
			return true
		}
	}
	return false
}

func emitDiagnostics(
	em diagnostic.Emitter,
	subject event.Subject,
	err error,
) diagnosticResult {
	var res diagnosticResult

	if err == nil {
		return res
	}

	var dp diagnostic.DiagnosticProvider
	if !errors.As(err, &dp) {
		return res
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

	return res
}
