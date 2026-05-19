// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// TestPass is emitted when an assertion (or `expect` matcher) passes.
type TestPass struct {
	Description string
}

func (e TestPass) Error() string { return e.Description }

func (e TestPass) Diagnostic() event.Event {
	return event.Info{
		Template: event.Template{
			ID:   CodeTestPass,
			Text: "{{.Description}}",
			Data: e,
		},
	}
}

// TestFail is emitted when an assertion (or `expect` matcher) fails.
type TestFail struct {
	Description string
	Expected    string
	Actual      string
	Source      spec.SourceSpan
}

func (e TestFail) Error() string { return e.Description }

func (e TestFail) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeTestFail,
			Text:   "{{.Description}}",
			Hint:   "expected: {{.Expected}}\nactual:   {{.Actual}}",
			Data:   e,
			Source: &e.Source,
		},
	}
}

// TestSummary is emitted at the end of a test file run with the
// total pass/fail counts.
type TestSummary struct {
	File   string
	Passed int
	Failed int
}

func (e TestSummary) Error() string { return e.File }

func (e TestSummary) Diagnostic() event.Event {
	return event.Info{
		Template: event.Template{
			ID:   CodeTestSummary,
			Text: "{{.File}}: {{.Passed}} passed, {{.Failed}} failed",
			Data: e,
		},
	}
}

// TestError is emitted when a test cannot be evaluated at all
// (infrastructure failure: file not found, broken setup, ...).
type TestError struct {
	Detail string
	Hint   string
}

func (e TestError) Error() string { return e.Detail }

func (e TestError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeTestError,
			Text: "{{.Detail}}",
			Hint: "{{.Hint}}",
			Data: e,
		},
	}
}
