package diagnostic

import (
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/render"
	"godoit.dev/doit/signal"
)

type (
	Policy struct {
		WarningsAsErrors bool
		Verbosity        signal.Verbosity
	}
	policyEmitter struct {
		pol Policy
		out render.Displayer
	}
)

func NewEmitter(policy Policy, displayer render.Displayer) Emitter {
	return &policyEmitter{
		pol: policy,
		out: displayer,
	}
}

func (p Policy) apply(s signal.Severity) signal.Severity {
	if s == signal.Warning && p.WarningsAsErrors {
		return signal.Error
	}

	return s
}

func (p *policyEmitter) Emit(ev event.Event) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.Emit(ev)
}
