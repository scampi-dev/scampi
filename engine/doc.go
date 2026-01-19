// Package engine implements the core execution model of doit.
//
// It is responsible for planning, checking, executing, and reporting actions
// and operations under explicit, fail-fast semantics. Execution state is
// authoritative; diagnostics are observational and must not influence control
// flow except where explicitly defined.
// ----
// package engine
// Package engine implements fail-fast, deterministic execution of plans.
//
// Execution semantics:
//   - Checks are pure, pessimistic, and dependency-independent
//   - Diagnostics decide abort; errors do not
//   - Execution is intentionally fail-fast
//   - Execution state is authoritative; diagnostics are observational
//
// Non-goals:
//   - Rollback or transactional execution
//   - Speculation or retries
//   - Recovery or compensation
//   ----------------------

// Package engine implements fail-fast, deterministic execution of plans.
//
// Execution semantics:
//   - Checks are pure, pessimistic, and dependency-independent
//   - Diagnostics decide abort; errors do not
//   - Execution is intentionally fail-fast
//   - Execution state is authoritative; diagnostics are observational
//
// Non-goals:
//   - Rollback or transactional execution
//   - Speculative or optimistic execution
//   - Retries, recovery, or compensation
package engine
