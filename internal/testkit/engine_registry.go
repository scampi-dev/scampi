// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"reflect"

	"scampi.dev/scampi/internal/spec"
)

// BaseRegistry is the lookup surface the test runner needs from a
// base engine registry. Exactly the shape of linker.Registry —
// duplicated here so testkit doesn't have to import the linker
// package (testkit lives below linker in the dependency stack).
type BaseRegistry interface {
	StepKind(kind string) (spec.StepKind, bool)
	TargetKind(kind string) (spec.TargetKind, bool)
	ConverterFor(reflect.Type) (spec.TypeConverter, bool)
}

// EngineRegistry wraps a BaseRegistry and overlays the two test
// target types — `test.target_in_memory` and `test.target_rest_mock`
// — both bound to the supplied TestRegistry so every constructor
// call during link registers itself for later verification.
//
// Step type lookups pass through to the base unchanged. Target type
// lookups check the test types first, falling back to the base for
// anything else (so real targets like ssh.target / local.target
// still work in test files that mix them with mocks, though that's
// uncommon in practice).
type EngineRegistry struct {
	base  BaseRegistry
	tests *TestRegistry
}

// NewEngineRegistry returns a registry that overlays test target
// types on top of base, all bound to tests.
func NewEngineRegistry(base BaseRegistry, tests *TestRegistry) *EngineRegistry {
	return &EngineRegistry{base: base, tests: tests}
}

// StepKind delegates to the base registry — no overlay.
func (r *EngineRegistry) StepKind(kind string) (spec.StepKind, bool) {
	return r.base.StepKind(kind)
}

// ConverterFor delegates to the base registry.
func (r *EngineRegistry) ConverterFor(t reflect.Type) (spec.TypeConverter, bool) {
	return r.base.ConverterFor(t)
}

// TargetKind returns the in-memory test target type, otherwise falls
// back to the base registry.
func (r *EngineRegistry) TargetKind(kind string) (spec.TargetKind, bool) {
	switch kind {
	case "test.target_in_memory":
		return MemTargetKind{Registry: r.tests}, true
	}
	return r.base.TargetKind(kind)
}
