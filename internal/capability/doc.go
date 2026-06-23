// SPDX-License-Identifier: GPL-3.0-only

// Package capability defines execution guarantees used during planning.
//
// A capability represents a concrete guarantee that a target provides and that a
// step may require. Capabilities are checked at plan time.
//
// Formally, capabilities are elements of a finite set. A step declares a set of
// required capabilities R. A target declares a set of provided capabilities P.
// A step is valid for a target if and only if:
//
//	R ⊆ P
//
// Capabilities are atomic and compositional. Higher-level concepts are expressed
// as named sets composed from smaller capabilities, not via inheritance or
// hierarchy.
//
// Example:
//
//	R = {Filesystem, Ownership, FileMode}
//	P = {Filesystem, Ownership, FileMode, Symlink}
//	⇒ planning succeeds because R ⊆ P
//
//	R = {Symlink}
//	P = {Filesystem}
//	⇒ planning fails because Symlink ∉ P
//
// The capability model supports conjunction only; disjunction and negation are
// intentionally not supported.
package capability
