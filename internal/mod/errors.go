// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"fmt"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
)

// ParseError
// -----------------------------------------------------------------------------

// ParseError is raised when scampi.mod cannot be parsed or contains invalid values.
type ParseError struct {
	Detail string
	Hint   string
	Span   spec.Span
}

func (e ParseError) Error() string {
	if e.Span.StartLine > 0 {
		return fmt.Sprintf("%s:%d: %s", e.Span.Filename, e.Span.StartLine, e.Detail)
	}
	if e.Span.Filename != "" {
		return fmt.Sprintf("%s: %s", e.Span.Filename, e.Detail)
	}
	return e.Detail
}

func (e ParseError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeParseError,
			Text: "{{.Detail}}",
			Hint: "{{.Hint}}",
			Data: e,
			Span: &e.Span,
		},
	}
}
