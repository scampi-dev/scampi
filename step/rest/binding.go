// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"

	"scampi.dev/scampi/errs"
)

// JQBinding extracts a value from a JSON document via a compiled jq expression.
// Unlike JQCheck (which tests truthiness), JQBinding captures the actual result
// for use in path interpolation and other bindings.
type JQBinding struct {
	Expr     string
	Compiled *gojq.Code
}

// bare-error: sentinel for binding extraction failures
var errBinding = errs.New("binding extraction")

// Extract runs the compiled jq expression against input and returns the first
// non-null, non-error result. Returns an error if the expression produces no
// usable value.
func (b *JQBinding) Extract(input any) (any, error) {
	iter := b.Compiled.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return nil, errs.WrapErrf(errBinding, "%s: %v", b.Expr, err)
		}
		if v != nil && v != false {
			return v, nil
		}
	}
	return nil, errs.WrapErrf(errBinding, "%s: no result", b.Expr)
}

// ResolveBindings evaluates each binding against the query result and returns
// a map of name → string value for path interpolation.
func ResolveBindings(bindings map[string]*JQBinding, input any) (map[string]string, error) {
	resolved := make(map[string]string, len(bindings))
	for name, binding := range bindings {
		val, err := binding.Extract(input)
		if err != nil {
			return nil, errs.WrapErrf(errBinding, "binding %q: %v", name, err)
		}
		resolved[name] = formatBindingValue(val)
	}
	return resolved, nil
}

// InterpolatePath replaces {name} placeholders in a path template with values
// from the resolved bindings map.
func InterpolatePath(template string, resolved map[string]string) string {
	oldnew := make([]string, 0, len(resolved)*2)
	for k, v := range resolved {
		oldnew = append(oldnew, "{"+k+"}", v)
	}
	return strings.NewReplacer(oldnew...).Replace(template)
}

func formatBindingValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case json.Number:
		return val.String()
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
