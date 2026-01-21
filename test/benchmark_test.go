package test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

/*
BENCHMARK PHILOSOPHY

These benchmarks are:
- regression guards, not speed contests
- stable across machines
- focused on hot paths only

They intentionally avoid:
- CLI rendering
- ANSI output
- disk-heavy work beyond config loading
*/

// -----------------------------------------------------------------------------
// Benchmark: loadConfig (schema + CUE validation)
// -----------------------------------------------------------------------------

func BenchmarkLoadConfig(b *testing.B) {
	tmp := b.TempDir()

	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfg := fmt.Sprintf(`
package bench

import (
  "list"
	"godoit.dev/doit/builtin"
)

units: [
  for i in list.Range(0, %d, 1) {
    builtin.copy & {
      name:  "unit-\(i)"
      src:   "/tmp/src-\(i)"
      dest:  "/tmp/dest-\(i)"
      perm:  "0644"
      owner: "user"
      group: "group"
    }
  }
]
`, size)
			cfgPath := absPath(filepath.Join(tmp, "config.cue"))
			writeOrDie(cfgPath, []byte(cfg), 0o644)

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				_, err := engine.LoadConfig(context.Background(), em, cfgPath, store)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Benchmark: diagnostic emission overhead
// -----------------------------------------------------------------------------

func BenchmarkDiagnosticEmission(b *testing.B) {
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	sub := event.Subject{
		Index: 0,
		Kind:  "copy",
		Name:  "bench",
	}

	diag := fakeDiagnostic{
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
	}

	for b.Loop() {
		em.Emit(diagnostic.DiagnosticRaised(sub, diag))
	}
}

// -----------------------------------------------------------------------------
// Benchmark: Apply() no-op run (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfg := fmt.Sprintf(`
package bench

import (
	"list"
	"godoit.dev/doit/builtin"
)

units: [
	for i in list.Range(0, %d, 1) {
		builtin.copy & {
			name:  "unit-\(i)"
			src:   "/src.txt"
			dest:  "/dest.txt"
			perm:  "0644"
			owner: "perf-owner"
			group: "perf-group"
		}
	}
]
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/src.txt"] = []byte("hello")
			tgt.Files["/dest.txt"] = []byte("hello")
			src.Files["/config.cue"] = []byte(cfg)

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()
			e := engine.New(src, tgt, em)

			for b.Loop() {
				if err := e.Apply(context.Background(), "/config.cue", store); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
