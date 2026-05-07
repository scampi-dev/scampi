// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const resourceSetID = "step.rest.resource_set"

// itemCategory describes a declared or remote item's reconciliation fate.
type itemCategory uint8

const (
	itemMissing   itemCategory = iota // declared but not remote
	itemDrift                         // matched, fields differ
	itemConverged                     // matched, no diff
	itemOrphan                        // remote but not declared
)

// categorizedItem pairs a key with its declared/remote data and category.
type categorizedItem struct {
	key      string
	category itemCategory
	declared map[string]any // nil for orphans
	remote   any            // nil for missing
	drift    []spec.DriftDetail
}

type resourceSetOp struct {
	sharedop.BaseOp
	query        *RequestConfig
	keyJQ        *JQCheck
	orphanFilter *JQCheck
	items        []map[string]any
	missing      *RequestConfig
	found        *RequestConfig
	orphan       *RequestConfig
	bindings     map[string]*JQBinding
	orphanState  map[string]any
	redact       []compiledRedact

	// Set during Check, consumed during Execute.
	plan        []categorizedItem
	queryResult any
}

func (op *resourceSetOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	client := target.Must[target.HTTPClient](resourceSetID, tgt)

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

	applyRedact(ctx, op.redact, body)

	// Extract the array of remote items via the query's jq check.
	remoteItems := extractJQAll(jqCheck.Compiled, body)
	op.queryResult = remoteItems

	// Build key → remote item index.
	remoteByKey := make(map[string]any, len(remoteItems))
	for _, item := range remoteItems {
		key, err := extractKey(op.keyJQ.Compiled, item)
		if err != nil {
			continue // skip items whose key can't be extracted
		}
		remoteByKey[key] = item
	}

	// Categorize each declared item.
	matchedKeys := make(map[string]bool, len(op.items))
	for _, declared := range op.items {
		key, err := extractKey(op.keyJQ.Compiled, declared)
		if err != nil {
			return spec.CheckUnsatisfied, nil, errs.WrapErrf(
				errResourceQuery,
				"extracting key from declared item: %v",
				err,
			)
		}
		matchedKeys[key] = true

		remote, exists := remoteByKey[key]
		if !exists {
			op.plan = append(op.plan, categorizedItem{
				key:      key,
				category: itemMissing,
				declared: declared,
			})
			continue
		}

		drift := diffState(declared, remote)
		if len(drift) == 0 {
			op.plan = append(op.plan, categorizedItem{
				key:      key,
				category: itemConverged,
				declared: declared,
				remote:   remote,
			})
		} else {
			op.plan = append(op.plan, categorizedItem{
				key:      key,
				category: itemDrift,
				declared: declared,
				remote:   remote,
				drift:    drift,
			})
		}
	}

	// Detect orphans (remote items not in declared set).
	if op.orphan != nil {
		for key, remote := range remoteByKey {
			if matchedKeys[key] {
				continue
			}
			if op.orphanFilter != nil && extractJQ(op.orphanFilter.Compiled, remote) == nil {
				continue // filter doesn't match — not an orphan
			}
			op.plan = append(op.plan, categorizedItem{
				key:      key,
				category: itemOrphan,
				remote:   remote,
			})
		}
	}

	// Build aggregate drift report.
	var allDrift []spec.DriftDetail
	hasChanges := false
	for _, ci := range op.plan {
		switch ci.category {
		case itemMissing:
			hasChanges = true
			allDrift = append(allDrift, spec.DriftDetail{
				Field: ci.key, Current: "missing", Desired: "present",
			})
		case itemDrift:
			hasChanges = true
			for _, d := range ci.drift {
				allDrift = append(allDrift, spec.DriftDetail{
					Field: ci.key + "." + d.Field, Current: d.Current, Desired: d.Desired,
				})
			}
		case itemOrphan:
			hasChanges = true
			allDrift = append(allDrift, spec.DriftDetail{
				Field: ci.key, Current: "present", Desired: "orphan",
			})
		}
	}

	if !hasChanges {
		return spec.CheckSatisfied, traceDrift(tgt), nil
	}

	allDrift = append(allDrift, traceDrift(tgt)...)
	return spec.CheckUnsatisfied, allDrift, nil
}

func (op *resourceSetOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	client := target.Must[target.HTTPClient](resourceSetID, tgt)

	changed := false
	for _, ci := range op.plan {
		switch ci.category {
		case itemMissing:
			if op.missing == nil {
				continue
			}
			if err := op.executeMissing(ctx, client, ci); err != nil {
				return spec.Result{}, err
			}
			changed = true

		case itemDrift:
			if op.found == nil {
				continue
			}
			if err := op.executeFound(ctx, client, ci); err != nil {
				return spec.Result{}, err
			}
			changed = true

		case itemOrphan:
			if op.orphan == nil {
				continue
			}
			if err := op.executeOrphan(ctx, client, ci); err != nil {
				return spec.Result{}, err
			}
			changed = true
		}
	}

	return spec.Result{Changed: changed}, nil
}

func (op *resourceSetOp) executeMissing(
	ctx context.Context,
	client target.HTTPClient,
	ci categorizedItem,
) error {
	bodyBytes, bodyHeaders, err := marshalState(ci.declared)
	if err != nil {
		return err
	}
	headers := mergeHeaders(bodyHeaders, op.missing.Headers)

	resp, err := client.Do(ctx, target.HTTPRequest{
		Method:  op.missing.Method,
		Path:    op.missing.Path,
		Headers: headers,
		Body:    bodyBytes,
	})
	if err != nil {
		return HTTPError{Phase: "execute", Method: op.missing.Method, Path: op.missing.Path, Err: err}
	}
	if resp.StatusCode >= 400 {
		return RequestError{
			Method: op.missing.Method, Path: op.missing.Path,
			Status: resp.StatusCode, Body: string(resp.Body),
		}
	}
	return nil
}

func (op *resourceSetOp) executeFound(
	ctx context.Context,
	client target.HTTPClient,
	ci categorizedItem,
) error {
	path := op.found.Path
	if len(op.bindings) > 0 {
		resolved, err := ResolveBindings(op.bindings, ci.remote)
		if err != nil {
			return HTTPError{Phase: "execute", Method: op.found.Method, Path: path, Err: err}
		}
		path = InterpolatePath(path, resolved)
	}

	bodyBytes, bodyHeaders, err := marshalState(ci.declared)
	if err != nil {
		return err
	}
	headers := mergeHeaders(bodyHeaders, op.found.Headers)

	resp, err := client.Do(ctx, target.HTTPRequest{
		Method:  op.found.Method,
		Path:    path,
		Headers: headers,
		Body:    bodyBytes,
	})
	if err != nil {
		return HTTPError{Phase: "execute", Method: op.found.Method, Path: path, Err: err}
	}
	if resp.StatusCode >= 400 {
		return RequestError{
			Method: op.found.Method, Path: path,
			Status: resp.StatusCode, Body: string(resp.Body),
		}
	}
	return nil
}

func (op *resourceSetOp) executeOrphan(
	ctx context.Context,
	client target.HTTPClient,
	ci categorizedItem,
) error {
	path := op.orphan.Path
	if len(op.bindings) > 0 {
		resolved, err := ResolveBindings(op.bindings, ci.remote)
		if err != nil {
			return HTTPError{Phase: "execute", Method: op.orphan.Method, Path: path, Err: err}
		}
		path = InterpolatePath(path, resolved)
	}

	bodyBytes, bodyHeaders, err := marshalState(op.orphanState)
	if err != nil {
		return err
	}
	headers := mergeHeaders(bodyHeaders, op.orphan.Headers)

	resp, err := client.Do(ctx, target.HTTPRequest{
		Method:  op.orphan.Method,
		Path:    path,
		Headers: headers,
		Body:    bodyBytes,
	})
	if err != nil {
		return HTTPError{Phase: "execute", Method: op.orphan.Method, Path: path, Err: err}
	}
	if resp.StatusCode >= 400 {
		return RequestError{
			Method: op.orphan.Method, Path: path,
			Status: resp.StatusCode, Body: string(resp.Body),
		}
	}
	return nil
}

// marshalState encodes a state map as JSON body bytes with appropriate headers.
func marshalState(state map[string]any) ([]byte, map[string]string, error) {
	if len(state) == 0 {
		return nil, nil, nil
	}
	body := JSONBody{Data: state}
	b, err := body.Bytes()
	if err != nil {
		return nil, nil, HTTPError{Phase: "execute", Err: err}
	}
	return b, body.Headers(), nil
}

// Output implements spec.OutputProvider.
func (op *resourceSetOp) Output() any { return op.queryResult }

func (resourceSetOp) RequiredCapabilities() capability.Capability {
	return capability.REST
}

// extractJQAll runs a compiled jq program against input and collects all
// non-null, non-error results. Used by resource_set to get the full remote
// set from a query.
func extractJQAll(code *gojq.Code, input any) []any {
	var results []any
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if _, isErr := v.(error); isErr {
			continue
		}
		if v != nil && v != false {
			results = append(results, v)
		}
	}
	return results
}

// extractKey runs the key jq expression against an item and returns a string
// key for matching. Uses formatBindingValue to stringify the jq output.
func extractKey(code *gojq.Code, item any) (string, error) {
	iter := code.Run(item)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if _, isErr := v.(error); isErr {
			continue
		}
		if v != nil && v != false {
			return formatBindingValue(v), nil
		}
	}
	return "", errs.WrapErrf(errResourceQuery, "key expression produced no result")
}

// OpDescription
// -----------------------------------------------------------------------------

type resourceSetOpDesc struct {
	Desc  string
	Query string
	Items int
}

func (d resourceSetOpDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   resourceSetID,
		Text: `{{if .Desc}}{{.Desc}}{{else}}resource set {{.Query}} ({{.Items}} items){{end}}`,
		Data: d,
	}
}

func (op *resourceSetOp) OpDescription() spec.OpDescription {
	return resourceSetOpDesc{
		Desc:  op.Action().Desc(),
		Query: op.query.Method + " " + op.query.Path,
		Items: len(op.items),
	}
}

func (op *resourceSetOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "query", Value: op.query.Method + " " + op.query.Path},
		{Label: "items", Value: fmt.Sprintf("%d", len(op.items))},
	}
	if op.missing != nil {
		fields = append(fields, spec.InspectField{Label: "missing", Value: op.missing.Method + " " + op.missing.Path})
	}
	if op.found != nil {
		fields = append(fields, spec.InspectField{Label: "found", Value: op.found.Method + " " + op.found.Path})
	}
	if op.orphan != nil {
		fields = append(fields, spec.InspectField{Label: "orphan", Value: op.orphan.Method + " " + op.orphan.Path})
	}
	return fields
}
