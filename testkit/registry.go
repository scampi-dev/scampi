// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"sync"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/target"
)

// TestRegistry holds all mock targets constructed during a single
// test run, paired with the `expect` value tree carried by their
// constructor calls. The runner builds a fresh registry per run,
// hands it to the engine via target type bindings, and walks it
// after engine.Apply to verify each entry.
type TestRegistry struct {
	mu         sync.Mutex
	memTargets []MemTargetEntry
	memRESTs   []MemRESTEntry
}

// MemTargetEntry pairs an in-memory POSIX mock with the expectations
// declared on its constructor call.
type MemTargetEntry struct {
	Name   string
	Mock   *target.MemTarget
	Expect *eval.StructVal // the `expect` field, may be nil
}

// MemRESTEntry pairs an in-memory REST mock with the request
// expectations declared on its constructor call.
type MemRESTEntry struct {
	Name           string
	Mock           *target.MemREST
	ExpectRequests *eval.ListVal // the `expect_requests` field, may be nil
}

// NewTestRegistry returns a fresh empty registry.
func NewTestRegistry() *TestRegistry {
	return &TestRegistry{}
}

// AddMemTarget records a new in-memory POSIX mock with its
// expectations. Called by MemTargetType.Create during link.
func (r *TestRegistry) AddMemTarget(e MemTargetEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.memTargets = append(r.memTargets, e)
}

// AddMemREST records a new in-memory REST mock with its
// expectations. Called by MemRESTTargetType.Create during link.
func (r *TestRegistry) AddMemREST(e MemRESTEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.memRESTs = append(r.memRESTs, e)
}

// MemTargets returns a snapshot of every in-memory POSIX mock
// registered so far. Used by the verifier after engine.Apply.
func (r *TestRegistry) MemTargets() []MemTargetEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]MemTargetEntry, len(r.memTargets))
	copy(out, r.memTargets)
	return out
}

// MemRESTs returns a snapshot of every in-memory REST mock
// registered so far. Used by the verifier after engine.Apply.
func (r *TestRegistry) MemRESTs() []MemRESTEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]MemRESTEntry, len(r.memRESTs))
	copy(out, r.memRESTs)
	return out
}
