package test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"godoit.dev/doit/diagnostic"
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

steps: [
  for i in list.Range(0, %d, 1) {
    builtin.copy & {
      desc:  "step-\(i)"
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

			src := source.LocalPosixSource{}
			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				_, err := engine.LoadConfig(context.Background(), em, cfgPath, store, src)
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

	diag := fakeDiagnostic{
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
	}

	for b.Loop() {
		em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(0, "copy", "bench", diag))
	}
}

// -----------------------------------------------------------------------------
// Benchmark: Apply() no-op run (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`
package bench

import (
	"list"
	"godoit.dev/doit/builtin"
)

steps: [
	for i in list.Range(0, %d, 1) {
		builtin.copy & {
			desc:  "step-\(i)"
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
			src.Files["/config.cue"] = []byte(cfgStr)

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx := context.Background()
				cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				cfg.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, cfg, noopEmitter{})
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}
				defer e.Close()

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Benchmark: Apply() no-op run for symlink (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Symlink(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`
package bench

import (
	"list"
	"godoit.dev/doit/builtin"
)

steps: [
	for i in list.Range(0, %d, 1) {
		builtin.symlink & {
			desc:   "symlink-\(i)"
			target: "/target.txt"
			link:   "/link.txt"
		}
	}
]
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.cue"] = []byte(cfgStr)
			tgt.Symlinks["/link.txt"] = "/target.txt"

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx := context.Background()
				cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				e, err := engine.New(ctx, source.LocalPosixSource{}, cfg, noopEmitter{})
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}
				defer e.Close()

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Benchmark: Apply() no-op run with mixed step types
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Mixed(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			// Half copy, half symlink
			cfg := fmt.Sprintf(`
package bench

import (
	"list"
	"godoit.dev/doit/builtin"
)

steps: [
	for i in list.Range(0, %d, 1) {
		builtin.copy & {
			desc:  "copy-\(i)"
			src:   "/src.txt"
			dest:  "/dest.txt"
			perm:  "0644"
			owner: "perf-owner"
			group: "perf-group"
		}
	},
	for i in list.Range(0, %d, 1) {
		builtin.symlink & {
			desc:   "symlink-\(i)"
			target: "/target.txt"
			link:   "/link.txt"
		}
	}
]
`, size, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/src.txt"] = []byte("hello")
			src.Files["/config.cue"] = []byte(cfg)
			tgt.Files["/dest.txt"] = []byte("hello")
			tgt.Symlinks["/link.txt"] = "/target.txt"

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()
			for b.Loop() {
				ctx := context.Background()
				cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				e, err := engine.New(ctx, source.LocalPosixSource{}, cfg, noopEmitter{})
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}
				defer e.Close()

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkValidateCueInput(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			steps := make([]string, 0)
			for i := range size {
				step := fmt.Sprintf(`
builtin.copy & {
  desc:  "step-%d"
	src:   "/src.txt"
	dest:  "/dest.txt"
	perm:  "0644"
	owner: "perf-owner"
	group: "perf-group"
}
`, i)
				steps = append(steps, step)
			}

			cfg := fmt.Sprintf(`
package bench

import (
	"list"
	"godoit.dev/doit/builtin"
)

steps: [
%s
]
`, strings.Join(steps, "\n\n"))

			data := []byte(cfg)
			for i := 0; i < b.N; i++ {
				_ = engine.ValidateCueInput(data)
			}
		})
	}
}
