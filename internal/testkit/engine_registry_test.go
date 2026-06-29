// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"reflect"
	"testing"

	"scampi.dev/scampi/internal/spec"
)

// stubBase is a tiny BaseRegistry for the wrapper tests. It records
// nothing and lets us assert which lookups were forwarded.
type stubBase struct {
	stepCalls   []string
	targetCalls []string
	targetTypes map[string]spec.TargetKind
}

func (s *stubBase) StepKind(kind string) (spec.StepKind, bool) {
	s.stepCalls = append(s.stepCalls, kind)
	return nil, false
}

func (s *stubBase) TargetKind(kind string) (spec.TargetKind, bool) {
	s.targetCalls = append(s.targetCalls, kind)
	if t, ok := s.targetTypes[kind]; ok {
		return t, true
	}
	return nil, false
}

func (s *stubBase) ConverterFor(reflect.Type) (spec.TypeConverter, bool) {
	return nil, false
}

func TestEngineRegistry_OverlaysTestTargets(t *testing.T) {
	base := &stubBase{}
	tests := NewTestRegistry()
	r := NewEngineRegistry(base, tests)

	// Both test target kinds resolve to test types without
	// touching the base registry.
	if tt, ok := r.TargetKind("test.target_in_memory"); !ok {
		t.Errorf("test.target_in_memory not found")
	} else if _, ok := tt.(MemTargetKind); !ok {
		t.Errorf("got %T, want MemTargetKind", tt)
	}

	if len(base.targetCalls) != 0 {
		t.Errorf("base lookup should not be called for test targets, got %v", base.targetCalls)
	}
}

func TestEngineRegistry_FallsThroughForOtherTargets(t *testing.T) {
	base := &stubBase{}
	r := NewEngineRegistry(base, NewTestRegistry())

	if _, ok := r.TargetKind("ssh"); ok {
		t.Errorf("ssh should not resolve through stub base")
	}
	if len(base.targetCalls) != 1 || base.targetCalls[0] != "ssh" {
		t.Errorf("base.TargetKind not called once with 'ssh', got %v", base.targetCalls)
	}
}

func TestEngineRegistry_StepKindPassthrough(t *testing.T) {
	base := &stubBase{}
	r := NewEngineRegistry(base, NewTestRegistry())

	r.StepKind("posix.copy")
	if len(base.stepCalls) != 1 || base.stepCalls[0] != "posix.copy" {
		t.Errorf("step lookup not forwarded: %v", base.stepCalls)
	}
}

func TestEngineRegistry_TestTypesShareRegistry(t *testing.T) {
	tests := NewTestRegistry()
	r := NewEngineRegistry(&stubBase{}, tests)

	tt, _ := r.TargetKind("test.target_in_memory")
	mtt := tt.(MemTargetKind)
	if mtt.Registry != tests {
		t.Errorf("MemTargetKind bound to wrong registry")
	}
}
