package engine

import (
	"fmt"

	cueerr "cuelang.org/go/cue/errors"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type CueDiagnostic struct {
	Err   cueerr.Error
	Phase string // "load", "validate", "decode"

	Source *spec.SourceSpan
}

func (d CueDiagnostic) Error() string {
	return fmt.Sprintf("cue.error in phase %q: %v", d.Phase, d.Err)
}

func (d CueDiagnostic) Diagnostics(subject event.Subject) []event.Event {
	var events []event.Event

	for _, e := range cueerr.Errors(d.Err) {
		events = append(events,
			diagnostic.DiagnosticRaised(subject, CueDiagnostic{
				Err:   e,
				Phase: d.Phase,
			}),
		)
	}

	return events
}

func (d CueDiagnostic) EventTemplate() event.Template {
	var src *spec.SourceSpan

	if d.Source != nil {
		src = d.Source
	} else if sp := spanFromPos(d.Err.Position()); sp.Line != 0 {
		src = &sp
	}

	msg := cueerr.Sanitize(d.Err).Error()

	if !d.Err.Position().IsValid() {
		if p := cueerr.Path(d.Err); len(p) > 0 {
			msg = fmt.Sprintf("%s (%#v)", msg, p)
		}
	}

	return event.Template{
		ID:     "cue." + d.Phase,
		Text:   msg,
		Source: src,
	}
}

func (d CueDiagnostic) Severity() signal.Severity {
	return signal.Error
}

type CueMissingField struct {
	Field      string
	UnitIndex  int
	UnitKind   string
	UnitName   string
	UnitSource spec.SourceSpan
}

type (
	MissingFieldDiagnostic struct {
		Missing CueMissingField
	}
)

func (d MissingFieldDiagnostic) Error() string {
	return fmt.Sprintf("field %q is mandatory", d.Missing.Field)
}

func (d MissingFieldDiagnostic) EventTemplate() event.Template {
	m := d.Missing
	return event.Template{
		ID:   "config.MissingField",
		Text: d.Error(),
		Source: &spec.SourceSpan{
			Filename: m.UnitSource.Filename,
			Line:     m.UnitSource.Line,
			StartCol: m.UnitSource.StartCol,
			EndCol:   m.UnitSource.EndCol,
		},
	}
}

func (d MissingFieldDiagnostic) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, d),
	}
}

func (MissingFieldDiagnostic) Severity() signal.Severity { return signal.Error }
func (MissingFieldDiagnostic) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

type InvalidUnitsShape struct {
	Source spec.SourceSpan
	Have   string
	Want   string
}

func (e InvalidUnitsShape) Error() string {
	return fmt.Sprintf("invalid units declaration: have %q, want %q", e.Have, e.Want)
}

func (e InvalidUnitsShape) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(
			subject,
			e,
		),
	}
}

func (e InvalidUnitsShape) EventTemplate() event.Template {
	return event.Template{
		ID:     "core.InvalidUnitsShape",
		Text:   "invalid 'units' declaration, expected {{.Want}}",
		Hint:   "expected {{.Want}}, have {{.Have}}",
		Help:   "the 'units' field must be a list, e.g. units: [ {...}, {...} ]",
		Source: &e.Source,
		Data:   e,
	}
}

func (e InvalidUnitsShape) Severity() signal.Severity {
	return signal.Error
}

func (InvalidUnitsShape) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}
