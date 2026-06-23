// SPDX-License-Identifier: GPL-3.0-only

// Package target defines mutable system effects.
//
// Targets perform side-effecting operations during execution. They are invoked
// only by the engine and must not contain planning, checking, or diagnostic
// policy logic.
package target
