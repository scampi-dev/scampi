// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"scampi.dev/scampi/errs"
)

var requestAssertionAttrs = []string{
	"body_contains",
	"header_equals",
	"was_called",
	"was_called_times",
	"was_not_called",
}

// RequestAssertion is the Starlark value returned by assert_that.request(method, path).
type RequestAssertion struct {
	mock      *MockREST
	method    string
	path      string
	collector *Collector
}

func (a *RequestAssertion) String() string {
	return fmt.Sprintf("request_assertion(%s %s)", a.method, a.path)
}
func (a *RequestAssertion) Type() string          { return "request_assertion" }
func (a *RequestAssertion) Freeze()               {}
func (a *RequestAssertion) Truth() starlark.Bool  { return starlark.True }
func (a *RequestAssertion) Hash() (uint32, error) { return 0, nil }
func (a *RequestAssertion) AttrNames() []string   { return requestAssertionAttrs }

func (a *RequestAssertion) Attr(name string) (starlark.Value, error) {
	switch name {
	case "was_called":
		return starlark.NewBuiltin("request.was_called", a.builtinWasCalled), nil
	case "was_called_times":
		return starlark.NewBuiltin("request.was_called_times", a.builtinWasCalledTimes), nil
	case "was_not_called":
		return starlark.NewBuiltin("request.was_not_called", a.builtinWasNotCalled), nil
	case "body_contains":
		return starlark.NewBuiltin("request.body_contains", a.builtinBodyContains), nil
	case "header_equals":
		return starlark.NewBuiltin("request.header_equals", a.builtinHeaderEquals), nil
	}
	return nil, nil
}

func (a *RequestAssertion) builtinWasCalled(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("was_called", args, kwargs, 0); err != nil {
		return nil, err
	}
	mock, method, path := a.mock, a.method, a.path
	a.collector.Add(Assertion{
		Description: fmt.Sprintf("%s %s was called", method, path),
		Check: func() error {
			if len(mock.CallsMatching(method, path)) == 0 {
				// bare-error: assertion result
				return errs.Errorf("%s %s was never called", method, path)
			}
			return nil
		},
	})
	return starlark.None, nil
}

func (a *RequestAssertion) builtinWasCalledTimes(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var n int
	if err := starlark.UnpackPositionalArgs("was_called_times", args, kwargs, 1, &n); err != nil {
		return nil, err
	}
	mock, method, path := a.mock, a.method, a.path
	a.collector.Add(Assertion{
		Description: fmt.Sprintf("%s %s was called %d time(s)", method, path, n),
		Check: func() error {
			got := len(mock.CallsMatching(method, path))
			if got != n {
				// bare-error: assertion result
				return errs.Errorf("%s %s called %d time(s), want %d", method, path, got, n)
			}
			return nil
		},
	})
	return starlark.None, nil
}

func (a *RequestAssertion) builtinWasNotCalled(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("was_not_called", args, kwargs, 0); err != nil {
		return nil, err
	}
	mock, method, path := a.mock, a.method, a.path
	a.collector.Add(Assertion{
		Description: fmt.Sprintf("%s %s was not called", method, path),
		Check: func() error {
			if n := len(mock.CallsMatching(method, path)); n > 0 {
				// bare-error: assertion result
				return errs.Errorf("%s %s was called %d time(s), expected 0", method, path, n)
			}
			return nil
		},
	})
	return starlark.None, nil
}

func (a *RequestAssertion) builtinBodyContains(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var substring string
	if err := starlark.UnpackPositionalArgs("body_contains", args, kwargs, 1, &substring); err != nil {
		return nil, err
	}
	mock, method, path := a.mock, a.method, a.path
	a.collector.Add(Assertion{
		Description: fmt.Sprintf("%s %s body contains %q", method, path, substring),
		Check: func() error {
			calls := mock.CallsMatching(method, path)
			if len(calls) == 0 {
				// bare-error: assertion result
				return errs.Errorf("%s %s was never called", method, path)
			}
			last := calls[len(calls)-1]
			if !strings.Contains(string(last.Body), substring) {
				// bare-error: assertion result
				return errs.Errorf(
					"%s %s body does not contain %q\ngot: %s",
					method, path, substring, string(last.Body),
				)
			}
			return nil
		},
	})
	return starlark.None, nil
}

func (a *RequestAssertion) builtinHeaderEquals(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name, value string
	if err := starlark.UnpackPositionalArgs("header_equals", args, kwargs, 2, &name, &value); err != nil {
		return nil, err
	}
	mock, method, reqPath := a.mock, a.method, a.path
	a.collector.Add(Assertion{
		Description: fmt.Sprintf("%s %s header %s=%s", method, reqPath, name, value),
		Check: func() error {
			calls := mock.CallsMatching(method, reqPath)
			if len(calls) == 0 {
				// bare-error: assertion result
				return errs.Errorf("%s %s was never called", method, reqPath)
			}
			last := calls[len(calls)-1]
			got := last.Headers[name]
			if got != value {
				// bare-error: assertion result
				return errs.Errorf(
					"%s %s header %q = %q, want %q",
					method, reqPath, name, got, value,
				)
			}
			return nil
		},
	})
	return starlark.None, nil
}
