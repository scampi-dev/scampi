// Package local defines targets that operate on the local system.
//
// Local targets perform side-effecting operations against the host system the
// engine is running on (for example, the local filesystem or process
// environment). They provide concrete execution contexts used during planning
// and execution.
//
// Like all targets, local targets are invoked only by the engine and must not
// contain planning, validation, or diagnostic policy logic.
package local
