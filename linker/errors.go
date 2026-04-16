// SPDX-License-Identifier: GPL-3.0-only

package linker

import "scampi.dev/scampi/errs"

// Diagnostic codes for linker-specific errors.
const (
	CodeUnresolved errs.Code = "link.Unresolved"
)

// UnresolvedError is returned when a stub declaration has no matching
// entry in the engine registry.
type UnresolvedError struct {
	Kind string // "step", "target", "type"
	Name string
}

func (e *UnresolvedError) Error() string      { return "unresolved " + e.Kind + ": " + e.Name }
func (e *UnresolvedError) GetCode() errs.Code { return CodeUnresolved }
