// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"context"
	"fmt"
	"maps"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const requestID = "builtin.rest.request"

type requestOp struct {
	sharedops.BaseOp
	method  string
	path    string
	headers map[string]string
	body    BodyConfig
	check   CheckConfig
}

func (op *requestOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	if op.check == nil {
		return spec.CheckUnsatisfied, nil, nil
	}

	client := target.Must[target.HTTPClient](requestID, tgt)

	method := op.check.CheckMethod()
	if method == "" {
		method = "GET"
	}
	path := op.check.CheckPath()
	if path == "" {
		path = op.path
	}

	resp, err := client.Do(ctx, target.HTTPRequest{
		Method:  method,
		Path:    path,
		Headers: op.headers,
	})
	if err != nil {
		return spec.CheckUnsatisfied, nil, HTTPError{
			Phase: "check", Method: method, Path: path, Err: err,
		}
	}

	satisfied, err := op.check.Evaluate(resp.StatusCode, resp.Body)
	if err != nil {
		return spec.CheckUnsatisfied, nil, HTTPError{
			Phase: "check", Method: method, Path: path, Err: err,
		}
	}

	if satisfied {
		return spec.CheckSatisfied, nil, nil
	}

	return spec.CheckUnsatisfied, checkDrift(op.check, resp.StatusCode), nil
}

func (op *requestOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	client := target.Must[target.HTTPClient](requestID, tgt)

	headers := make(map[string]string)
	maps.Copy(headers, op.headers)

	var body []byte
	if op.body != nil {
		var err error
		body, err = op.body.Bytes()
		if err != nil {
			return spec.Result{}, HTTPError{
				Phase: "execute", Method: op.method, Path: op.path, Err: err,
			}
		}
		for k, v := range op.body.Headers() {
			if _, exists := headers[k]; !exists {
				headers[k] = v
			}
		}
	}

	resp, err := client.Do(ctx, target.HTTPRequest{
		Method:  op.method,
		Path:    op.path,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		return spec.Result{}, HTTPError{
			Phase: "execute", Method: op.method, Path: op.path, Err: err,
		}
	}

	if resp.StatusCode >= 400 {
		return spec.Result{}, RequestError{
			Method: op.method,
			Path:   op.path,
			Status: resp.StatusCode,
			Body:   string(resp.Body),
		}
	}

	return spec.Result{Changed: true}, nil
}

func checkDrift(check CheckConfig, statusCode int) []spec.DriftDetail {
	switch c := check.(type) {
	case StatusCheck:
		return []spec.DriftDetail{{
			Field:   "status",
			Current: fmt.Sprintf("%d", statusCode),
			Desired: fmt.Sprintf("%d", c.Status),
		}}
	case *JQCheck:
		return []spec.DriftDetail{{
			Field:   "jq",
			Current: "no match",
			Desired: c.Expr,
		}}
	default:
		return []spec.DriftDetail{{
			Field:   "check",
			Current: "unsatisfied",
			Desired: check.Kind(),
		}}
	}
}

func (requestOp) RequiredCapabilities() capability.Capability {
	return capability.REST
}

// OpDescription
// -----------------------------------------------------------------------------

type requestOpDesc struct {
	Method string
	Path   string
}

func (d requestOpDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   requestID,
		Text: `{{.Method}} {{.Path}}`,
		Data: d,
	}
}

func (op *requestOp) OpDescription() spec.OpDescription {
	return requestOpDesc{
		Method: op.method,
		Path:   op.path,
	}
}

func (op *requestOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "method", Value: op.method},
		{Label: "path", Value: op.path},
	}
	if op.check != nil {
		fields = append(fields, spec.InspectField{Label: "check", Value: op.check.Kind()})
	}
	return fields
}
