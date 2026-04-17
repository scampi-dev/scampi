// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const requestID = "step.rest.request"

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
		return spec.CheckSatisfied, traceDrift(tgt), nil
	}

	drift := checkDrift(op.check, method, path, resp)
	drift = append(drift, traceDrift(tgt)...)
	return spec.CheckUnsatisfied, drift, nil
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

func checkDrift(check CheckConfig, method, path string, resp *target.HTTPResponse) []spec.DriftDetail {
	var drift []spec.DriftDetail

	// V: primary check failure reason
	switch c := check.(type) {
	case StatusCheck:
		drift = append(drift, spec.DriftDetail{
			Field:   "status",
			Current: fmt.Sprintf("%d", resp.StatusCode),
			Desired: fmt.Sprintf("%d", c.Status),
		})
	case *JQCheck:
		drift = append(drift, spec.DriftDetail{
			Field:   "jq",
			Current: "no match",
			Desired: c.Expr,
		})
		drift = append(drift, spec.DriftDetail{
			Field:   "status",
			Current: fmt.Sprintf("%d", resp.StatusCode),
		})
	default:
		drift = append(drift, spec.DriftDetail{
			Field:   "check",
			Current: "unsatisfied",
			Desired: check.Kind(),
		})
	}

	// VV: response body
	drift = append(drift, responseDrift(resp.Body)...)

	// VVV: request/response metadata
	drift = append(drift, metaDrift(method, path, resp)...)

	return drift
}

func responseDrift(body []byte) []spec.DriftDetail {
	if len(body) == 0 {
		return nil
	}

	pretty := prettyJSON(body)
	if len(pretty) > 2048 {
		pretty = pretty[:2048] + "..."
	}

	return []spec.DriftDetail{{
		Field:     "body",
		Current:   pretty,
		Verbosity: signal.VV,
	}}
}

func metaDrift(method, path string, resp *target.HTTPResponse) []spec.DriftDetail {
	drift := []spec.DriftDetail{{
		Field:     "request",
		Current:   fmt.Sprintf("%s %s", method, path),
		Verbosity: signal.VVV,
	}}

	for _, h := range flattenHeaders(resp.Headers) {
		drift = append(drift, spec.DriftDetail{
			Field:     h.name,
			Current:   h.value,
			Verbosity: signal.VVV,
		})
	}

	return drift
}

func traceDrift(tgt target.Target) []spec.DriftDetail {
	tr, ok := tgt.(target.Traceable)
	if !ok {
		return nil
	}
	traces := tr.DrainTraces()
	if len(traces) == 0 {
		return nil
	}
	var drift []spec.DriftDetail
	for _, msg := range traces {
		drift = append(drift, spec.DriftDetail{
			Field:     "trace",
			Current:   msg,
			Verbosity: signal.VVV,
		})
	}
	return drift
}

func prettyJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "         ", "  "); err != nil {
		return strings.TrimSpace(string(data))
	}
	return buf.String()
}

type headerPair struct {
	name  string
	value string
}

func flattenHeaders(h map[string][]string) []headerPair {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []headerPair
	for _, k := range keys {
		for _, v := range h[k] {
			out = append(out, headerPair{name: k, value: v})
		}
	}
	return out
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
