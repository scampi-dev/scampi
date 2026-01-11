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
		Errs    []error
		Effects []diagnostic.Effect
	}
	diagnosticResults []diagnosticResult
)

func (r *diagnosticResult) add(effect diagnostic.Effect) {
	r.Effects = append(r.Effects, effect)
}

func (r diagnosticResult) ShouldAbort() bool {
	for _, e := range r.Effects {
		if e == diagnostic.EffectAbort {
			return true
		}
	}
	return false
}

func (r *diagnosticResults) Append(dr diagnosticResult) {
	*r = append(*r, dr)
}

func (r diagnosticResults) ShouldAbort() bool {
	for _, dr := range r {
		if dr.ShouldAbort() {
			return true
		}
	}
	return false
}

func (r diagnosticResults) Errs() []error {
	var res []error
	for _, dr := range r {
		res = append(res, dr.Errs...)
	}
	return res
}

func emitDiagnostics(
	em diagnostic.Emitter,
	subject event.Subject,
	err error,
) diagnosticResult {
	var res diagnosticResult
	res.Errs = append(res.Errs, err)

	if err == nil {
		return res
	}

	var dp diagnostic.DiagnosticProvider
	if !errors.As(err, &dp) {
		return res
	}

	for _, ev := range dp.Diagnostics(subject) {
		em.Emit(ev)

		// determine effect
		effect := diagnostic.EffectAbort // default (safe)
		if ep, ok := err.(diagnostic.EffectProvider); ok {
			effect = ep.Effect()
		}

		res.add(effect)
	}

	return res
}
