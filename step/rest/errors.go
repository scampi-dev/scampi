// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

type RequestError struct {
	diagnostic.FatalError
	Method string
	Path   string
	Status int
	Body   string
}

func (e RequestError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.Status, e.Body)
}

func (e RequestError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeRequestError,
		Text: "{{.Method}} {{.Path}}: status {{.Status}}",
		Hint: `the {{.Method}} returned status {{.Status}} — verify the request body ` +
			`and that the endpoint accepts {{.Method}}`,
		Help: "{{.Body}}",
		Data: e,
	}
}

type HTTPError struct {
	diagnostic.FatalError
	Phase  string // "check" or "execute"
	Method string
	Path   string
	Err    error
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("%s %s %s: %s", e.Phase, e.Method, e.Path, e.Err)
}

func (e HTTPError) Unwrap() error { return e.Err }

func (e HTTPError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeHTTPError,
		Text: "{{.Phase}} {{.Method}} {{.Path}} failed",
		Hint: `verify the REST target is reachable and the {{.Method}} on "{{.Path}}" is supported`,
		Help: "{{.Err}}",
		Data: e,
	}
}

// RedactPathError flags an invalid jq path supplied to a request's
// `redact` list at plan time.
type RedactPathError struct {
	diagnostic.FatalError
	Path   string
	Err    error
	Source spec.SourceSpan
}

func (e RedactPathError) Error() string {
	return fmt.Sprintf("invalid redact path %q: %s", e.Path, e.Err)
}

func (e RedactPathError) Unwrap() error { return e.Err }

func (e RedactPathError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeRedactPathError,
		Text: `invalid redact path: {{.Path}}`,
		Hint: `redact paths use jq syntax with an optional leading dot — ` +
			`e.g. "x_ssh_password", "data.token", or "items[0].secret"`,
		Help:   "{{.Err}}",
		Data:   e,
		Source: &e.Source,
	}
}

// Resource errors
// -----------------------------------------------------------------------------

type ResourceQueryError struct {
	diagnostic.FatalError
	Method string
	Path   string
	Err    error
}

func (e ResourceQueryError) Error() string {
	return fmt.Sprintf("resource query %s %s: %s", e.Method, e.Path, e.Err)
}

func (e ResourceQueryError) Unwrap() error { return e.Err }

func (e ResourceQueryError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeResourceQueryError,
		Text: "resource query {{.Method}} {{.Path}} failed",
		Hint: `verify the REST target is reachable and that "{{.Path}}" is a queryable resource endpoint`,
		Help: "{{.Err}}",
		Data: e,
	}
}
