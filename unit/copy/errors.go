package copy

import (
	"fmt"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type InvalidOctal struct {
	Value    string
	Regex    string
	Examples []string

	Source spec.SourceSpan
	Err    error
}

func (e InvalidOctal) Error() string {
	return fmt.Sprintf("invalid octal %q", e.Value)
}

func (e InvalidOctal) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e InvalidOctal) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.InvalidOctal",
		Text: "invalid octal permission '{{.Value}}'",
		Hint: `expected {{.Regex}} (examples: {{join ", " .Examples}})`,
		Help: "file permissions must be specified using valid octal notation",
		Data: e,
	}
}

func (e InvalidOctal) Severity() signal.Severity {
	return signal.Error
}
