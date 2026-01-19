// Package template provides a thin wrapper around text/template rendering.
//
// It is used by renderers to expand small formatting templates with helper
// functions. Templates are parsed and executed at render time; failures are
// treated as invariant violations and currently panic.
//
// This package does not define rendering policy or event semantics.
package template
