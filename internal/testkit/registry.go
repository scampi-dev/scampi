// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"sync"

	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/target"
)

// TestRegistry holds all mock targets constructed during a single
// test run, paired with the `expect` value tree carried by their
// constructor calls. The runner builds a fresh registry per run,
// hands it to the engine via target type bindings, and walks it
// after engine.Apply to verify each entry.
type TestRegistry struct {
	mu         sync.Mutex
	memTargets []MemTargetEntry
}

// MemTargetEntry pairs an in-memory POSIX mock with the expectations
// declared on its constructor call.
type MemTargetEntry struct {
	Name   string
	Mock   *target.MemTarget
	Expect *eval.StructVal // the `expect` field, may be nil
}

// NewTestRegistry returns a fresh empty registry.
func NewTestRegistry() *TestRegistry {
	return &TestRegistry{}
}

// AddMemTarget records a new in-memory POSIX mock with its
// expectations. Called by MemTargetKind.Create during link.
//
// Returns the canonical entry for the given name — either a fresh
// one (newly registered) or the existing entry if one is already
// registered with that name. Multi-deploy tests rely on this: each
// engine.New call wraps a separate Config and triggers its
// own MemTargetKind.Create, but we want all of them to share the
// same MemTarget so the verifier sees the combined post-apply state.
func (r *TestRegistry) AddMemTarget(e MemTargetEntry) MemTargetEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.memTargets {
		if existing.Name == e.Name {
			return existing
		}
	}
	r.memTargets = append(r.memTargets, e)
	return e
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
