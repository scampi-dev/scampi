// Package render defines presentation logic for diagnostic events.
//
// Renderers consume diagnostic events and transform them into user-facing
// output. Rendering is side-effectful but non-semantic: it must not influence
// execution, diagnostics policy, or control flow.
package render
