// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/itchyny/gojq"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// stepOutputs stores settled state from executed steps, keyed by step ID.
// Written after a step completes, read before dependent steps execute.
type stepOutputs struct {
	mu   sync.Mutex
	data map[spec.StepID]any
}

func newStepOutputs() *stepOutputs {
	return &stepOutputs{data: make(map[spec.StepID]any)}
}

func (s *stepOutputs) Store(id spec.StepID, output any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = output
}

func (s *stepOutputs) Load(id spec.StepID) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[id]
	return v, ok
}

// refPending is a sentinel value returned by the check-mode resolver when
// the referenced step has no output yet (would change). Downstream steps
// see this as an unresolvable value, causing drift detection to report
// "would change" rather than erroring.
type refPending struct{}

// buildRefResolver creates a spec.RefResolver that looks up step outputs
// and evaluates jq expressions against them. In check mode, missing outputs
// return a pending sentinel instead of erroring.
func buildRefResolver(outputs *stepOutputs, checkMode bool) spec.RefResolver {
	return func(ref spec.Ref) (any, error) {
		src := &ref.Source

		out, ok := outputs.Load(ref.TargetID)
		if !ok {
			if checkMode {
				return refPending{}, nil
			}
			return nil, RefError{
				Expr:   ref.Expr,
				Detail: "referenced step has no output — is it included in the steps list?",
				Source: src,
			}
		}

		query, err := gojq.Parse(ref.Expr)
		if err != nil {
			return nil, RefError{Expr: ref.Expr, Detail: fmt.Sprintf("invalid jq: %v", err), Source: src}
		}
		code, err := gojq.Compile(query)
		if err != nil {
			return nil, RefError{Expr: ref.Expr, Detail: fmt.Sprintf("jq compile: %v", err), Source: src}
		}

		iter := code.Run(out)
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if jqErr, isErr := v.(error); isErr {
				return nil, RefError{Expr: ref.Expr, Detail: fmt.Sprintf("jq: %v", jqErr), Source: src}
			}
			if v != nil && v != false {
				return normalizeJQValue(v), nil
			}
		}
		return nil, RefError{
			Expr:   ref.Expr,
			Detail: "expression produced no result",
			Source: src,
		}
	}
}

// normalizeJQValue converts json.Number to float64 for consistency with
// the rest of the value pipeline.
func normalizeJQValue(v any) any {
	if n, ok := v.(json.Number); ok {
		if f, err := n.Float64(); err == nil {
			return f
		}
		return n.String()
	}
	return v
}

// Engine-level interfaces for actions that support refs.

// stepIdentifier exposes a step's unique ID for output registry keying.
type stepIdentifier interface {
	StepID() spec.StepID
}

// refResolvable marks actions whose configs contain ref() markers that
// must be resolved before execution.
type refResolvable interface {
	ResolveRefs(spec.RefResolver) error
}

// RefError
// -----------------------------------------------------------------------------

type RefError struct {
	diagnostic.FatalError
	Expr   string
	Detail string
	Source *spec.SourceSpan
}

func (e RefError) Error() string {
	return fmt.Sprintf("ref(%s): %s", e.Expr, e.Detail)
}

func (e RefError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeRefError,
		Text:   "ref({{.Expr}}) failed",
		Hint:   "{{.Detail}}",
		Data:   e,
		Source: e.Source,
	}
}
