// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"slices"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/spec"
)

// IndexAll returns the catalog of all registered step types, sorted
// by Kind ascending so the caller can render deterministically.
func IndexAll(_ context.Context) []spec.StepDoc {
	reg := NewRegistry()
	types := reg.StepTypes()
	docs := make([]spec.StepDoc, 0, len(types))
	for _, t := range types {
		docs = append(docs, loadStepDoc(reg, t.Kind()))
	}
	slices.SortStableFunc(docs, func(a, b spec.StepDoc) int {
		return strings.Compare(a.Kind, b.Kind)
	})
	return docs
}

// IndexStep returns the documentation for a single step kind. If
// stepKind isn't registered, fires UnknownIndexKindError as a
// diagnostic through em and returns AbortError so the caller can
// shape the exit code.
func IndexStep(_ context.Context, stepKind string, em diagnostic.Emitter) (spec.StepDoc, error) {
	reg := NewRegistry()
	_, ok := reg.StepType(stepKind)
	if !ok {
		em.Emit(UnknownIndexKindError{Kind: stepKind}.Diagnostic())
		return spec.StepDoc{}, AbortError{}
	}
	return loadStepDoc(reg, stepKind), nil
}
