// SPDX-License-Identifier: GPL-3.0-only

// Package integration holds engine-level integration tests and the
// benchmark suite consumed by the scampi.dev dashboard.
//
// Each `func BenchmarkXxx(b *testing.B, ...)` in this package MUST
// have a doc comment describing what it measures; the description
// is rendered next to the chart on the live dashboard. Regenerate
// the published descriptions via `just generate` after adding or
// modifying a benchmark.
//
//go:generate go run ../../bin/bench-descriptions -o ../../site/data/benchmarks.json
package integration
