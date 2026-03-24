// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
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
		ID:   "rest.RequestError",
		Text: "{{.Method}} {{.Path}}: status {{.Status}}",
		Hint: "the API returned an error response",
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
		ID:   "rest.HTTPError",
		Text: "{{.Phase}} {{.Method}} {{.Path}} failed",
		Hint: "{{.Err}}",
		Data: e,
	}
}
