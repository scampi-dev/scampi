// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"fmt"
	"path/filepath"
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
// Benchmark: loadConfig (Starlark evaluation)
// -----------------------------------------------------------------------------

func BenchmarkLoadConfig(b *testing.B) {
	tmp := b.TempDir()

	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfg := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        copy(
            desc="step-%%d" %% i,
            src="/tmp/src-%%d" %% i,
            dest="/tmp/dest-%%d" %% i,
            perm="0644",
            owner="user",
            group="group",
        )
        for i in range(%d)
    ],
)
`, size)

			cfgPath := absPath(filepath.Join(tmp, "config.star"))
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
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        copy(
            desc="step-%%d" %% i,
            src="/src.txt",
            dest="/dest.txt",
            perm="0644",
            owner="perf-owner",
            group="perf-group",
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/src.txt"] = []byte("hello")
			tgt.Files["/dest.txt"] = []byte("hello")
			src.Files["/config.star"] = []byte(cfgStr)

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx := context.Background()
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, noopEmitter{})
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
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
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        symlink(
            desc="symlink-%%d" %% i,
            target="/target.txt",
            link="/link.txt",
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Symlinks["/link.txt"] = "/target.txt"

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx := context.Background()
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, noopEmitter{})
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
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
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        copy(
            desc="copy-%%d" %% i,
            src="/src.txt",
            dest="/dest.txt",
            perm="0644",
            owner="perf-owner",
            group="perf-group",
        )
        for i in range(%d)
    ] + [
        symlink(
            desc="symlink-%%d" %% i,
            target="/target.txt",
            link="/link.txt",
        )
        for i in range(%d)
    ],
)
`, size, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/src.txt"] = []byte("hello")
			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Files["/dest.txt"] = []byte("hello")
			tgt.Symlinks["/link.txt"] = "/target.txt"

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()
			for b.Loop() {
				ctx := context.Background()
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, noopEmitter{})
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
			}
		})
	}
}
