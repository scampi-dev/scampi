package engine

import (
	"context"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/spec"
)

func IndexAll(_ context.Context, em diagnostic.Emitter) error {
	reg := NewRegistry()
	types := reg.StepTypes()

	docs := make([]spec.StepDoc, 0, len(types))
	for _, t := range types {
		docs = append(docs, LoadStepDoc(t.Kind()))
	}

	em.EmitIndexAll(diagnostic.IndexAllProduced(docs))
	return nil
}

func IndexStep(_ context.Context, stepKind string, em diagnostic.Emitter) error {
	reg := NewRegistry()
	_, ok := reg.StepType(stepKind)
	if !ok {
		emitIndexDiagnostic(em, UnknownIndexKind{Kind: stepKind})
		return AbortError{}
	}

	em.EmitIndexStep(diagnostic.IndexStepProduced(LoadStepDoc(stepKind)))
	return nil
}

func emitIndexDiagnostic(em diagnostic.Emitter, d diagnostic.Diagnostic) {
	em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic("", d))
}
