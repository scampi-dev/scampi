// SPDX-License-Identifier: GPL-3.0-only

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
