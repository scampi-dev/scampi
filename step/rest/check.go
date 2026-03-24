// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"encoding/json"

	"github.com/itchyny/gojq"

	"scampi.dev/scampi/errs"
)

// CheckConfig evaluates an HTTP response to determine if the desired state
// is already satisfied. Implementations are constructed in Starlark
// (rest.status, rest.jq) and stored in RequestConfig.Check.
type CheckConfig interface {
	// CheckMethod returns the HTTP method for the check request.
	// Empty string means GET.
	CheckMethod() string
	// CheckPath returns the path for the check request.
	// Empty string means use the execute request's path.
	CheckPath() string
	// Evaluate returns true if the response indicates the desired state exists.
	Evaluate(statusCode int, body []byte) (bool, error)
	Kind() string
}

// StatusCheck
// -----------------------------------------------------------------------------

type StatusCheck struct {
	Status int
	Path   string
	Method string
}

func (c StatusCheck) Kind() string        { return "status" }
func (c StatusCheck) CheckMethod() string { return c.Method }
func (c StatusCheck) CheckPath() string   { return c.Path }

func (c StatusCheck) Evaluate(statusCode int, _ []byte) (bool, error) {
	return statusCode == c.Status, nil
}

// JQCheck
// -----------------------------------------------------------------------------

// bare-error: sentinel for jq evaluation failures
var errJQ = errs.New("jq evaluation")

type JQCheck struct {
	Expr   string
	Path   string
	Method string

	Compiled *gojq.Code
}

func (c *JQCheck) Kind() string        { return "jq" }
func (c *JQCheck) CheckMethod() string { return c.Method }
func (c *JQCheck) CheckPath() string   { return c.Path }

func (c *JQCheck) Evaluate(statusCode int, body []byte) (bool, error) {
	if statusCode < 200 || statusCode >= 300 {
		return false, nil
	}

	var input any
	if err := json.Unmarshal(body, &input); err != nil {
		return false, errs.WrapErrf(errJQ, "parse response body: %v", err)
	}

	iter := c.Compiled.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return false, errs.WrapErrf(errJQ, "%s: %v", c.Expr, err)
		}
		// Any non-null, non-false output means satisfied.
		if v != nil && v != false {
			return true, nil
		}
	}
	return false, nil
}

// CompileJQ parses and compiles a jq expression. Called at Starlark eval time
// so syntax errors surface early.
func CompileJQ(expr string) (*gojq.Code, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, err
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, err
	}
	return code, nil
}
