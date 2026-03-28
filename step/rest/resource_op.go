// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"sort"

	"github.com/itchyny/gojq"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

// bare-error: sentinels for resource query failures
var (
	errResourceQuery = errs.New("resource query")
	errResourceParse = errs.New("resource parse")
)

const resourceID = "builtin.rest.resource"

type resourceMode uint8

const (
	resourceNoop resourceMode = iota
	resourceMissing
	resourceFound
)

type resourceOp struct {
	sharedops.BaseOp
	query    *RequestConfig
	missing  *RequestConfig
	found    *RequestConfig
	bindings map[string]*JQBinding
	state    map[string]any

	// Set during Check, consumed during Execute.
	queryResult any
	mode        resourceMode
}

func (op *resourceOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	client := target.Must[target.HTTPClient](resourceID, tgt)

	jqCheck := op.query.Check.(*JQCheck)

	method := jqCheck.CheckMethod()
	if method == "" {
		method = "GET"
	}
	path := jqCheck.CheckPath()
	if path == "" {
		path = op.query.Path
	}

	resp, err := client.Do(ctx, target.HTTPRequest{
		Method:  method,
		Path:    path,
		Headers: op.query.Headers,
	})
	if err != nil {
		return spec.CheckUnsatisfied, nil, ResourceQueryError{
			Method: method, Path: path, Err: err,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return spec.CheckUnsatisfied, nil, ResourceQueryError{
			Method: method, Path: path,
			Err: errs.WrapErrf(errResourceQuery, "status %d", resp.StatusCode),
		}
	}

	var body any
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return spec.CheckUnsatisfied, nil, ResourceQueryError{
			Method: method, Path: path,
			Err: errs.WrapErrf(errResourceParse, "%v", err),
		}
	}

	result := extractJQ(jqCheck.Compiled, body)

	// Resource not found.
	if result == nil {
		if op.missing == nil {
			op.mode = resourceNoop
			return spec.CheckSatisfied, nil, nil
		}
		op.mode = resourceMissing
		return spec.CheckUnsatisfied, []spec.DriftDetail{
			{Field: "resource", Current: "missing", Desired: "present"},
		}, nil
	}

	// Resource found.
	op.queryResult = result

	if op.found == nil {
		op.mode = resourceNoop
		return spec.CheckSatisfied, nil, nil
	}

	// No state → found fires unconditionally (e.g. delete-if-exists).
	if len(op.state) == 0 {
		op.mode = resourceFound
		return spec.CheckUnsatisfied, []spec.DriftDetail{
			{Field: "resource", Current: "present", Desired: "absent"},
		}, nil
	}

	// State provided → diff to detect drift.
	drift := diffState(op.state, result)
	if len(drift) == 0 {
		op.mode = resourceNoop
		return spec.CheckSatisfied, nil, nil
	}

	op.mode = resourceFound
	return spec.CheckUnsatisfied, drift, nil
}

func (op *resourceOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	client := target.Must[target.HTTPClient](resourceID, tgt)

	switch op.mode {
	case resourceMissing:
		return op.executeMissing(ctx, client)
	case resourceFound:
		return op.executeFound(ctx, client)
	default:
		return spec.Result{}, nil
	}
}

func (op *resourceOp) executeMissing(
	ctx context.Context,
	client target.HTTPClient,
) (spec.Result, error) {
	bodyBytes, bodyHeaders, err := op.stateBody()
	if err != nil {
		return spec.Result{}, err
	}

	headers := mergeHeaders(bodyHeaders, op.missing.Headers)

	resp, err := client.Do(ctx, target.HTTPRequest{
		Method:  op.missing.Method,
		Path:    op.missing.Path,
		Headers: headers,
		Body:    bodyBytes,
	})
	if err != nil {
		return spec.Result{}, HTTPError{
			Phase: "execute", Method: op.missing.Method, Path: op.missing.Path, Err: err,
		}
	}
	if resp.StatusCode >= 400 {
		return spec.Result{}, RequestError{
			Method: op.missing.Method,
			Path:   op.missing.Path,
			Status: resp.StatusCode,
			Body:   string(resp.Body),
		}
	}
	return spec.Result{Changed: true}, nil
}

func (op *resourceOp) executeFound(
	ctx context.Context,
	client target.HTTPClient,
) (spec.Result, error) {
	path := op.found.Path
	if len(op.bindings) > 0 {
		resolved, err := ResolveBindings(op.bindings, op.queryResult)
		if err != nil {
			return spec.Result{}, HTTPError{
				Phase: "execute", Method: op.found.Method, Path: path, Err: err,
			}
		}
		path = InterpolatePath(path, resolved)
	}

	bodyBytes, bodyHeaders, err := op.stateBody()
	if err != nil {
		return spec.Result{}, err
	}

	headers := mergeHeaders(bodyHeaders, op.found.Headers)

	resp, err := client.Do(ctx, target.HTTPRequest{
		Method:  op.found.Method,
		Path:    path,
		Headers: headers,
		Body:    bodyBytes,
	})
	if err != nil {
		return spec.Result{}, HTTPError{
			Phase: "execute", Method: op.found.Method, Path: path, Err: err,
		}
	}
	if resp.StatusCode >= 400 {
		return spec.Result{}, RequestError{
			Method: op.found.Method,
			Path:   path,
			Status: resp.StatusCode,
			Body:   string(resp.Body),
		}
	}
	return spec.Result{Changed: true}, nil
}

// stateBody returns the JSON-encoded state as a request body, or nil if no
// state is configured.
func (op *resourceOp) stateBody() ([]byte, map[string]string, error) {
	if len(op.state) == 0 {
		return nil, nil, nil
	}
	body := JSONBody{Data: op.state}
	b, err := body.Bytes()
	if err != nil {
		return nil, nil, HTTPError{Phase: "execute", Err: err}
	}
	return b, body.Headers(), nil
}

func mergeHeaders(base, override map[string]string) map[string]string {
	headers := maps.Clone(base)
	if headers == nil {
		headers = make(map[string]string)
	}
	maps.Copy(headers, override)
	return headers
}

func (resourceOp) RequiredCapabilities() capability.Capability {
	return capability.REST
}

// extractJQ runs a compiled jq program against input and returns the first
// non-null, non-false result. Returns nil if no result matches.
func extractJQ(code *gojq.Code, input any) any {
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if _, isErr := v.(error); isErr {
			return nil
		}
		if v != nil && v != false {
			return v
		}
	}
}

// State diffing
// -----------------------------------------------------------------------------

func diffState(desired map[string]any, current any) []spec.DriftDetail {
	currentMap, ok := current.(map[string]any)
	if !ok {
		return []spec.DriftDetail{{
			Field: "state", Current: fmt.Sprintf("(%T)", current), Desired: "object",
		}}
	}

	var drift []spec.DriftDetail
	keys := sortedKeys(desired)
	for _, key := range keys {
		desiredVal := desired[key]
		currentVal, exists := currentMap[key]
		if !exists {
			drift = append(drift, spec.DriftDetail{
				Field: key, Current: "<absent>", Desired: formatDriftValue(desiredVal),
			})
			continue
		}
		if !valuesEqual(desiredVal, currentVal) {
			drift = append(drift, spec.DriftDetail{
				Field: key, Current: formatDriftValue(currentVal), Desired: formatDriftValue(desiredVal),
			})
		}
	}
	return drift
}

func valuesEqual(desired, current any) bool {
	// Normalize int64 vs float64 (Starlark int → int64, JSON number → float64).
	if d, ok := asFloat64(desired); ok {
		if c, ok := asFloat64(current); ok {
			return d == c
		}
		return false
	}

	// Bool, string.
	if reflect.TypeOf(desired) == reflect.TypeOf(current) {
		switch d := desired.(type) {
		case string:
			return d == current.(string)
		case bool:
			return d == current.(bool)
		}
	}

	// Slices.
	dSlice, dOk := desired.([]any)
	cSlice, cOk := current.([]any)
	if dOk && cOk {
		if len(dSlice) != len(cSlice) {
			return false
		}
		for i := range dSlice {
			if !valuesEqual(dSlice[i], cSlice[i]) {
				return false
			}
		}
		return true
	}

	// Maps.
	dMap, dOk := desired.(map[string]any)
	cMap, cOk := current.(map[string]any)
	if dOk && cOk {
		if len(dMap) != len(cMap) {
			return false
		}
		for k, dv := range dMap {
			cv, exists := cMap[k]
			if !exists || !valuesEqual(dv, cv) {
				return false
			}
		}
		return true
	}

	return reflect.DeepEqual(desired, current)
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func formatDriftValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// OpDescription
// -----------------------------------------------------------------------------

type resourceOpDesc struct {
	Desc    string
	Query   string
	Missing string
	Found   string
}

func (d resourceOpDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   resourceID,
		Text: `{{if .Desc}}{{.Desc}}{{else}}resource {{.Query}}{{end}}`,
		Data: d,
	}
}

func (op *resourceOp) OpDescription() spec.OpDescription {
	desc := resourceOpDesc{
		Desc:  op.Action().Desc(),
		Query: op.query.Method + " " + op.query.Path,
	}
	if op.missing != nil {
		desc.Missing = op.missing.Method + " " + op.missing.Path
	}
	if op.found != nil {
		desc.Found = op.found.Method + " " + op.found.Path
	}
	return desc
}

func (op *resourceOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "query", Value: op.query.Method + " " + op.query.Path},
	}
	if op.missing != nil {
		fields = append(fields, spec.InspectField{Label: "missing", Value: op.missing.Method + " " + op.missing.Path})
	}
	if op.found != nil {
		fields = append(fields, spec.InspectField{Label: "found", Value: op.found.Method + " " + op.found.Path})
	}
	if len(op.state) > 0 {
		fields = append(fields, spec.InspectField{Label: "state keys", Value: fmt.Sprintf("%d", len(op.state))})
	}
	return fields
}
