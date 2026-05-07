// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"

	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/spec"
)

// compiledRedact is a parsed jq path the engine walks against a
// response body; matched string values are registered with the
// redactor on the request context.
type compiledRedact struct {
	expr string
	code *gojq.Code
}

// compileRedact compiles a list of user-supplied redact paths.
// The user-facing form is a dotted name with optional leading `.`
// (e.g. `x_ssh_password`, `data.tokens[0]`); we prepend `.` to make
// it a valid jq expression. Empty entries are skipped.
//
// Errors are typed at the rest.request source span — invalid paths
// surface as plan-time diagnostics, not runtime "command not found"
// style failures.
func compileRedact(paths []string, src spec.SourceSpan) ([]compiledRedact, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out := make([]compiledRedact, 0, len(paths))
	for _, raw := range paths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		expr := raw
		if !strings.HasPrefix(expr, ".") {
			expr = "." + expr
		}
		query, err := gojq.Parse(expr)
		if err != nil {
			return nil, RedactPathError{Path: raw, Err: err, Source: src}
		}
		code, err := gojq.Compile(query)
		if err != nil {
			return nil, RedactPathError{Path: raw, Err: err, Source: src}
		}
		out = append(out, compiledRedact{expr: expr, code: code})
	}
	return out, nil
}

// registerRedact parses raw JSON body bytes and walks every redact
// path; matched values are added to the redactor on ctx. Convenience
// wrapper for callers that have raw bytes — non-JSON bodies are
// silently skipped (a redact list against a non-JSON response is a
// noop, not an error).
func registerRedact(ctx context.Context, redact []compiledRedact, body []byte) {
	if len(redact) == 0 || len(body) == 0 {
		return
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return
	}
	applyRedact(ctx, redact, parsed)
}

// applyRedact walks every compiled redact path against body and
// registers string-shaped results with the redactor on ctx. A nil
// redactor (no secret.WithRedactor in the chain) is a no-op, so
// callers don't need to nil-check. Non-string results are coerced
// to strings via fmt.Sprintf("%v", ...) so number/bool secrets — if
// the user redacts those — still get masked in rendered output.
//
// Errors from gojq Run are swallowed: a path that doesn't match
// the response is not a failure — it just means there's nothing to
// redact at that path on this response.
func applyRedact(ctx context.Context, redact []compiledRedact, body any) {
	if len(redact) == 0 {
		return
	}
	r := secret.FromContext(ctx)
	if r == nil {
		return
	}
	for _, cr := range redact {
		walkRedact(cr.code, body, r)
	}
}

// walkRedact iterates every match of code against input and adds
// string-coerced values to r.
func walkRedact(code *gojq.Code, input any, r *secret.Redactor) {
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			return
		}
		if _, isErr := v.(error); isErr {
			continue
		}
		if v == nil {
			continue
		}
		switch s := v.(type) {
		case string:
			r.Add(s)
		default:
			r.Add(fmt.Sprintf("%v", s))
		}
	}
}
