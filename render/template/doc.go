// SPDX-License-Identifier: GPL-3.0-only

// Package template provides a thin wrapper around text/template rendering.
//
// Two distinct callers depend on it:
//   - render/cli expands diagnostic event templates (text, hint, help) into
//     user-facing strings.
//   - step/template expands user-supplied templates against scampi data when
//     rendering files on a target.
//
// Templates are parsed and executed at render time; failures are treated as
// invariant violations and currently panic.
//
// This package does not define rendering policy or event semantics.
package template
