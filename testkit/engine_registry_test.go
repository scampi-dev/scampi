// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"testing"

	"scampi.dev/scampi/spec"
)

// stubBase is a tiny BaseRegistry for the wrapper tests. It records
// nothing and lets us assert which lookups were forwarded.
type stubBase struct {
	stepCalls   []string
	targetCalls []string
	targetTypes map[string]spec.TargetType
}

func (s *stubBase) StepType(kind string) (spec.StepType, bool) {
	s.stepCalls = append(s.stepCalls, kind)
	return nil, false
}

func (s *stubBase) TargetType(kind string) (spec.TargetType, bool) {
	s.targetCalls = append(s.targetCalls, kind)
	if t, ok := s.targetTypes[kind]; ok {
		return t, true
	}
	return nil, false
}

func TestEngineRegistry_OverlaysTestTargets(t *testing.T) {
	base := &stubBase{}
	tests := NewTestRegistry()
	r := NewEngineRegistry(base, tests)

	// Both test target kinds resolve to test types without
	// touching the base registry.
	if tt, ok := r.TargetType("test.target_in_memory"); !ok {
		t.Errorf("test.target_in_memory not found")
	} else if _, ok := tt.(MemTargetType); !ok {
		t.Errorf("got %T, want MemTargetType", tt)
	}

	if tt, ok := r.TargetType("test.target_rest_mock"); !ok {
		t.Errorf("test.target_rest_mock not found")
	} else if _, ok := tt.(MemRESTTargetType); !ok {
		t.Errorf("got %T, want MemRESTTargetType", tt)
	}

	if len(base.targetCalls) != 0 {
		t.Errorf("base lookup should not be called for test targets, got %v", base.targetCalls)
	}
}

func TestEngineRegistry_FallsThroughForOtherTargets(t *testing.T) {
	base := &stubBase{}
	r := NewEngineRegistry(base, NewTestRegistry())

	if _, ok := r.TargetType("ssh"); ok {
		t.Errorf("ssh should not resolve through stub base")
	}
	if len(base.targetCalls) != 1 || base.targetCalls[0] != "ssh" {
		t.Errorf("base.TargetType not called once with 'ssh', got %v", base.targetCalls)
	}
}

func TestEngineRegistry_StepTypePassthrough(t *testing.T) {
	base := &stubBase{}
	r := NewEngineRegistry(base, NewTestRegistry())

	r.StepType("posix.copy")
	if len(base.stepCalls) != 1 || base.stepCalls[0] != "posix.copy" {
		t.Errorf("step lookup not forwarded: %v", base.stepCalls)
	}
}

func TestEngineRegistry_TestTypesShareRegistry(t *testing.T) {
	tests := NewTestRegistry()
	r := NewEngineRegistry(&stubBase{}, tests)

	tt, _ := r.TargetType("test.target_in_memory")
	mtt := tt.(MemTargetType)
	if mtt.Registry != tests {
		t.Errorf("MemTargetType bound to wrong registry")
	}

	tr, _ := r.TargetType("test.target_rest_mock")
	mtr := tr.(MemRESTTargetType)
	if mtr.Registry != tests {
		t.Errorf("MemRESTTargetType bound to wrong registry")
	}
}
