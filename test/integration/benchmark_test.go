// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/internal/controller"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

// benchSizes returns the scale-out sizes for a benchmark. Default is
// just {1} so the pre-commit gate's -benchtime=1x smoke runs in tens
// of milliseconds per bench instead of seconds. Set SCAMPI_BENCH_FULL=1
// to run the full set - `just test bench` does this automatically.
func benchSizes(full ...int) []int {
	if os.Getenv("SCAMPI_BENCH_FULL") != "" {
		return full
	}
	return []int{1}
}

// recordCmdsOp samples MemTarget.Commands before and after a bench
// loop, reporting the per-op delta as the "cmds/op" custom metric.
// benchstat tracks it as a first-class metric over time so a
// regression that adds extra shell commands per step (e.g. the
// identity-cache class of bug from #416) lands as a clear delta in
// `just test benchcomp` output instead of being lost in ns/op noise.
//
// Usage:
//
//	done := recordCmdsOp(b, tgt)
//	for b.Loop() { ... }
//	done()
func recordCmdsOp(b *testing.B, tgt *target.MemTarget) func() {
	start := len(tgt.Commands)
	return func() {
		delta := len(tgt.Commands) - start
		if b.N > 0 {
			b.ReportMetric(float64(delta)/float64(b.N), "cmds/op")
		}
	}
}

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

// Benchmark: loadConfig (scampi evaluation)
// -----------------------------------------------------------------------------

// Benchmark_LoadConfig_FullPipeline measures the full scampi load pipeline: lex,
// parse, resolve, evaluate. Tracks language-layer overhead per step
// count.
func Benchmark_LoadConfig_FullPipeline(b *testing.B) {
	tmp := b.TempDir()

	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfg := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.copy {
      desc = "step-${i}"
      src = posix.source_local { path = "/tmp/src-${i}" }
      dest = "/tmp/dest-${i}"
      perm = "0644"
      owner = "user"
      group = "group"
    }
  }
}
`, size)

			cfgPath := harness.AbsPath(filepath.Join(tmp, "config.scampi"))
			harness.WriteOrDie(cfgPath, []byte(cfg), 0o644)

			ctl := controller.Posix{}
			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), cfgPath, store, ctl)
				if err != nil {
					b.Fatal(err)
				}
				cancel()
			}
		})
	}
}

// Benchmark: diagnostic emission overhead
// -----------------------------------------------------------------------------

// Benchmark_Emitter_RaiseOverhead measures the cost of raising one
// diagnostic through the emitter pipeline. Catches regressions in
// event routing and template rendering.
func Benchmark_Emitter_RaiseOverhead(b *testing.B) {
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	diag := harness.NewFakeDiagnostic(signal.Error, diagnostic.ImpactAbort, nil)

	for b.Loop() {
		em.Raise(diag)
	}
}

// Benchmark: Apply() no-op run (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Copy measures the converged-state Apply path for
// posix.copy steps. Drift detection runs; Execute is skipped because
// the target already has the desired content.
func Benchmark_ApplyNoOp_Copy(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.copy {
      desc = "step-${i}"
      src = posix.source_local { path = "/src.txt" }
      dest = "/dest-${i}.txt"
      perm = "0644"
      owner = "perf-owner"
      group = "perf-group"
    }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/src.txt"] = []byte("hello")
			for j := range size {
				tgt.Files[fmt.Sprintf("/dest-%d.txt", j)] = []byte("hello")
			}
			ctl.Files["/config.scampi"] = []byte(cfgStr)

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for symlink (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Symlink is the symlink-step variant of
// Benchmark_ApplyNoOp_Copy - drift detection on pre-existing symlinks.
func Benchmark_ApplyNoOp_Symlink(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.symlink { target = "/target.txt", link = "/link-${i}.txt" }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				tgt.Symlinks[fmt.Sprintf("/link-%d.txt", j)] = "/target.txt"
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for dir (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Dir is the directory-step variant - drift
// detection on pre-existing dirs with matching mode/owner.
func Benchmark_ApplyNoOp_Dir(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.dir { path = "/mydir-${i}" }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				tgt.Dirs[fmt.Sprintf("/mydir-%d", j)] = 0o755
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run with mixed step types
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Mixed exercises a configuration with multiple
// step kinds at once (copy, dir, symlink, template). Heavier per-
// step work than the uniform _NoOp variants; catches scheduler /
// DAG regressions that single-kind benches miss.
func Benchmark_ApplyNoOp_Mixed(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.copy {
      desc = "copy-${i}"
      src = posix.source_local { path = "/src.txt" }
      dest = "/dest-${i}.txt"
      perm = "0644"
      owner = "perf-owner"
      group = "perf-group"
    }
  }
  for i in std.range(%d) {
    posix.symlink { target = "/target.txt", link = "/link-${i}.txt" }
  }
}
`, size, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/src.txt"] = []byte("hello")
			ctl.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				tgt.Files[fmt.Sprintf("/dest-%d.txt", j)] = []byte("hello")
				tgt.Symlinks[fmt.Sprintf("/link-%d.txt", j)] = "/target.txt"
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for template (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Template is the template-step variant - drift
// detection on rendered template output that already matches.
func Benchmark_ApplyNoOp_Template(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.template {
      desc = "tmpl-${i}"
      src = posix.source_inline { content = "server {{ .name }} port={{ .port }}" }
      dest = "/out-${i}.conf"
      perm = "0644"
      owner = "perf-owner"
      group = "perf-group"
      data = {"values": {"name": "bench", "port": 8080}}
    }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				tgt.Files[fmt.Sprintf("/out-%d.conf", j)] = []byte("server bench port=8080")
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for pkg (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Pkg is the package-step variant - drift
// detection on packages already installed via the MemTarget backend.
func Benchmark_ApplyNoOp_Pkg(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.pkg { packages = ["nginx"], source = posix.pkg_system {} }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			tgt.Pkgs["nginx"] = true

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for service (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Service is the service-step variant - drift
// detection on services already in the desired running/enabled state.
func Benchmark_ApplyNoOp_Service(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.service { name = "nginx", state = posix.ServiceState.running, enabled = true }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			tgt.Services["nginx"] = true
			tgt.EnabledServices["nginx"] = true

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for group (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Group is the group-step variant - drift detection
// on groups already present with the desired GID/members.
func Benchmark_ApplyNoOp_Group(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.group { name = "deploy-${i}" }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				name := fmt.Sprintf("deploy-%d", j)
				tgt.Groups[name] = target.GroupInfo{Name: name}
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for user (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_User is the user-step variant - drift detection on
// users already present with the desired shell/home/groups.
func Benchmark_ApplyNoOp_User(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.user { name = "deploy-${i}", shell = "/bin/bash", groups = ["sudo"] }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				name := fmt.Sprintf("deploy-%d", j)
				tgt.Users[name] = target.UserInfo{
					Name:   name,
					Shell:  "/bin/bash",
					Groups: []string{"sudo"},
				}
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for sysctl (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Sysctl is the sysctl-step variant - drift
// detection issues one `sysctl -n` per step, so cmds/op should scale
// linearly with step count.
func Benchmark_ApplyNoOp_Sysctl(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.sysctl { key = "net.ipv4.ip_forward", value = "1" }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			tgt.Files["/etc/sysctl.d/99-scampi-net-ipv4-ip_forward.conf"] = []byte("net.ipv4.ip_forward = 1\n")
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				if cmd == "sysctl -n net.ipv4.ip_forward" {
					return target.CommandResult{ExitCode: 0, Stdout: "1\n"}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for firewall (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Firewall is the firewall-step variant - drift
// detection on firewall rules already in place.
func Benchmark_ApplyNoOp_Firewall(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.firewall { port = 22 }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				switch cmd {
				case "ufw version":
					return target.CommandResult{ExitCode: 0, Stdout: "ufw 0.36.2\n"}, nil
				case "ufw show added":
					return target.CommandResult{
						ExitCode: 0,
						Stdout:   "Added user rules:\nufw allow 22/tcp\n",
					}, nil
				default:
					return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
				}
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for run step (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Run is the run-step variant - idempotency check
// returns success so the apply command is skipped.
func Benchmark_ApplyNoOp_Run(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.run { apply = "do-thing", check = "check-thing" }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				if cmd == "check-thing" {
					return target.CommandResult{ExitCode: 0}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for container.instance (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Container is the container.instance variant -
// drift detection on containers already running with the desired
// image / ports / env.
func Benchmark_ApplyNoOp_Container(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"
import "std/container"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    container.instance { name = "app-${i}", image = "nginx:1.25" }
  }
}
`, size)

			ctl := controller.NewMem()
			tgt := target.NewMemTarget()

			ctl.Files["/config.scampi"] = []byte(cfgStr)
			for i := range size {
				tgt.Containers[fmt.Sprintf("app-%d", i)] = target.ContainerInfo{
					Name: fmt.Sprintf("app-%d", i), Image: "nginx:1.25",
					Running: true, Restart: "unless-stopped",
				}
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for unarchive (idempotent path)
// -----------------------------------------------------------------------------
//
// Size-N is the number of 1KB files in the archive.

func generateFiles(n int) map[string]string {
	content := strings.Repeat("x", 1024)
	files := make(map[string]string, n)
	for i := range n {
		files[fmt.Sprintf("file-%04d.txt", i)] = content
	}
	return files
}

// Benchmark_ApplyNoOp_UnarchiveTarGz is the .tar.gz variant of the
// unarchive bench. Drift detection via the cached content hash; no
// re-extraction.
func Benchmark_ApplyNoOp_UnarchiveTarGz(b *testing.B) {
	benchUnarchiveNoOp(b, makeTarGz, "/data.tar.gz")
}

// Benchmark_ApplyNoOp_UnarchiveTarXz is the xz-compressed unarchive
// variant - same drift-detect shape as the .tar.gz bench.
func Benchmark_ApplyNoOp_UnarchiveTarXz(b *testing.B) {
	benchUnarchiveNoOp(b, makeTarXz, "/data.tar.xz")
}

// Benchmark_ApplyNoOp_UnarchiveTarZst is the zstd-compressed unarchive
// variant - same drift-detect shape as the .tar.gz bench.
func Benchmark_ApplyNoOp_UnarchiveTarZst(b *testing.B) {
	benchUnarchiveNoOp(b, makeTarZst, "/data.tar.zst")
}

// Benchmark_ApplyNoOp_UnarchiveTar is the uncompressed-tar unarchive
// variant - same drift-detect shape as the .tar.gz bench.
func Benchmark_ApplyNoOp_UnarchiveTar(b *testing.B) {
	benchUnarchiveNoOp(b, makeTar, "/data.tar")
}

// Benchmark_ApplyNoOp_UnarchiveZip is the .zip unarchive variant - same
// drift-detect shape as the .tar.gz bench.
func Benchmark_ApplyNoOp_UnarchiveZip(b *testing.B) {
	benchUnarchiveNoOp(b, makeZip, "/data.zip")
}

func benchUnarchiveNoOp(b *testing.B, makeFn func(testing.TB, map[string]string) []byte, srcPath string) {
	for _, n := range benchSizes(1, 10, 100, 1000) {
		archive := makeFn(b, generateFiles(n))
		b.Run(fmt.Sprintf("Size-%d", n), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  posix.unarchive {
    src = posix.source_local { path = "%s" }
    dest = "/output"
    depth = 0
  }
}
`, srcPath)

			ctl := controller.NewMem()
			ctl.Files[srcPath] = archive
			ctl.Files["/config.scampi"] = []byte(cfgStr)

			tgt := target.NewMemTarget()
			tgt.Files[destMarkerPath("/output")] = []byte(archiveHash(archive) + "\n")

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})

	}
}

// Benchmark: Apply() no-op run for mount (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Mount is the mount-step variant - drift detection
// on mounts already present in /etc/fstab and mounted. Capped at
// Size-1000 because findFstabEntry is O(N) per step -> O(N^2) total.
func Benchmark_ApplyNoOp_Mount(b *testing.B) {
	// Cap at 1000: each mount step reads the entire /etc/fstab to find
	// its line (O(N) per step), so 10000 steps is O(N^2) work that
	// pegs the runner for >1 minute without producing more signal than
	// 1000 already gives us.
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			var cfgEntries strings.Builder
			var fstab strings.Builder
			mountedTargets := make(map[string]bool, size)
			for i := range size {
				dest := fmt.Sprintf("/mnt/data-%d", i)
				src := fmt.Sprintf("/dev/sda%d", i+1)
				fmt.Fprintf(
					&cfgEntries,
					"  posix.mount { src = %q, dest = %q, fs_type = posix.MountType.ext4, opts = \"defaults\" }\n",
					src,
					dest,
				)
				fmt.Fprintf(&fstab, "%s %s ext4 defaults 0 0\n", src, dest)
				mountedTargets[dest] = true
			}

			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
%s}
`, cfgEntries.String())

			ctl := controller.NewMem()
			ctl.Files["/config.scampi"] = []byte(cfgStr)

			tgt := target.NewMemTarget()
			tgt.Files["/etc/fstab"] = []byte(fstab.String())
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				// findmnt --target <dest> --noheadings -> exit 0 means mounted.
				if after, ok := strings.CutPrefix(cmd, "findmnt --target "); ok {
					rest := after
					rest = strings.TrimSuffix(rest, " --noheadings")
					if mountedTargets[strings.TrimSpace(rest)] {
						return target.CommandResult{ExitCode: 0}, nil
					}
					return target.CommandResult{ExitCode: 1}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for run_set (idempotent path)
// -----------------------------------------------------------------------------

// Benchmark_ApplyNoOp_Runset is the run_set-step variant - set-diff
// logic where the list command already returns the desired items.
func Benchmark_ApplyNoOp_Runset(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			// Each run_set step lists exactly one item, and desired
			// matches it - no add, no remove, no drift.
			var cfgEntries strings.Builder
			for i := range size {
				fmt.Fprintf(
					&cfgEntries,
					"  posix.run_set { list = \"list-%[1]d\", add = \"add {{ item }}\", "+
						"remove = \"rm {{ item }}\", desired = [\"item-%[1]d\"] }\n",
					i,
				)
			}

			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
%s}
`, cfgEntries.String())

			ctl := controller.NewMem()
			ctl.Files["/config.scampi"] = []byte(cfgStr)

			tgt := target.NewMemTarget()
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				// list-N returns "item-N\n" so the desired set matches.
				if after, ok := strings.CutPrefix(cmd, "list-"); ok {
					n := after
					return target.CommandResult{ExitCode: 0, Stdout: "item-" + n + "\n"}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() cold run with mixed step types (mutation path)
// -----------------------------------------------------------------------------
//
// All other ApplyNoOp_* benches measure the converged-state Check path.
// This one measures the COLD path: target starts empty, Apply mutates.
// Each iteration creates a fresh target so iteration N+1 is also cold.
// The b.StopTimer fence keeps the reset out of the measured interval.

// Benchmark_ApplyMixed_Cold measures the cold mutation path:
// mixed-step config against a fresh target each iteration so
// Execute always runs. Complement to the converged _NoOp variants.
func Benchmark_ApplyMixed_Cold(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "bench", targets = [host]) {
  for i in std.range(%d) {
    posix.dir {
      path  = "/var/cold-${i}"
      perm  = "0755"
      owner = "perf-owner"
      group = "perf-group"
    }
    posix.symlink {
      target = "/var/cold-${i}/target"
      link   = "/var/cold-${i}/link"
    }
  }
}
`, size)

			ctl := controller.NewMem()
			ctl.Files["/config.scampi"] = []byte(cfgStr)

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewInputStore()

			var cmdTotal int
			for b.Loop() {
				b.StopTimer()
				tgt := target.NewMemTarget()
				b.StartTimer()

				ctx, cancel := context.WithCancel(b.Context())
				cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockDeclaredTarget(tgt)

				e, err := engine.New(diagnostic.NewCtx(ctx, em), ctl, resolved)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(diagnostic.NewCtx(ctx, em)); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()

				b.StopTimer()
				cmdTotal += len(tgt.Commands)
				b.StartTimer()
			}
			if b.N > 0 {
				b.ReportMetric(float64(cmdTotal)/float64(b.N), "cmds/op")
			}
		})
	}
}
