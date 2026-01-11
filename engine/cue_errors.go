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
}

func (d CueDiagnostic) Error() string {
	return d.Err.Error()
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

	if sp := spanFromPos(d.Err.Position()); sp.Line != 0 {
		src = &sp
	}
	// __AUTO_GENERATED_PRINT_VAR_START__
	fmt.Println(fmt.Sprintf("EventTemplate d.Err: %v", d.Err.Position())) // __AUTO_GENERATED_PRINT_VAR_END__

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
	MissingFieldsDiagnostic struct {
		Missing []CueMissingField
	}
	MissingFieldDiagnostic struct {
		Missing CueMissingField
	}
)

func (d MissingFieldsDiagnostic) Error() string {
	return "missing required fields"
}

func (d MissingFieldsDiagnostic) Diagnostics(subject event.Subject) []event.Event {
	var events []event.Event

	for _, m := range d.Missing {
		events = append(events,
			diagnostic.DiagnosticRaised(
				event.Subject{
					Index: m.UnitIndex,
					Kind:  m.UnitKind,
					Name:  m.UnitName,
				},
				MissingFieldDiagnostic{
					Missing: m,
				},
			),
		)
	}

	return events
}

func (d MissingFieldDiagnostic) EventTemplate() event.Template {
	m := d.Missing
	return event.Template{
		ID:   "cue.missing-field",
		Text: fmt.Sprintf("field %q is mandatory", m.Field),
		Source: &spec.SourceSpan{
			Filename: m.UnitSource.Filename,
			Line:     m.UnitSource.Line,
			StartCol: m.UnitSource.StartCol,
			EndCol:   m.UnitSource.EndCol,
		},
	}
}

func (MissingFieldDiagnostic) Severity() signal.Severity { return signal.Error }
func (MissingFieldDiagnostic) Effect() diagnostic.Effect { return diagnostic.EffectAbort }
