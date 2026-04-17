// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// TestPass is emitted when an assertion (or `expect` matcher) passes.
type TestPass struct {
	diagnostic.Info
	Description string
}

func (e TestPass) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeTestPass,
		Text: "{{.Description}}",
		Data: e,
	}
}

// TestFail is emitted when an assertion (or `expect` matcher) fails.
type TestFail struct {
	diagnostic.FatalError
	Description string
	Expected    string
	Actual      string
	Source      spec.SourceSpan
}

func (e TestFail) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeTestFail,
		Text:   "{{.Description}}",
		Hint:   "expected: {{.Expected}}\nactual:   {{.Actual}}",
		Data:   e,
		Source: &e.Source,
	}
}

// TestSummary is emitted at the end of a test file run with the
// total pass/fail counts.
type TestSummary struct {
	diagnostic.Info
	File   string
	Passed int
	Failed int
}

func (e TestSummary) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeTestSummary,
		Text: "{{.File}}: {{.Passed}} passed, {{.Failed}} failed",
		Data: e,
	}
}

// TestError is emitted when a test cannot be evaluated at all
// (infrastructure failure: file not found, broken setup, ...).
type TestError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
}

func (e TestError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeTestError,
		Text: "{{.Detail}}",
		Hint: "{{.Hint}}",
		Data: e,
	}
}
