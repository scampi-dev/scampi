// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/test/harness"
)

// benchSizes returns the scale-out sizes for a benchmark. Default is
// just {1} so the pre-commit gate's -benchtime=1x smoke runs in tens
// of milliseconds per bench instead of seconds. Set SCAMPI_BENCH_FULL=1
// to run the full set — `just test bench` does this automatically.
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

func BenchmarkLoadConfig(b *testing.B) {
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

			src := source.LocalPosixSource{}
			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				_, err := engine.LoadConfig(ctx, em, cfgPath, store, src)
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

func BenchmarkDiagnosticEmission(b *testing.B) {
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	diag := harness.NewFakeDiagnostic(signal.Error, diagnostic.ImpactAbort, nil)

	for b.Loop() {
		em.Raise(diag)
	}
}

// Benchmark: Apply() no-op run (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/src.txt"] = []byte("hello")
			for j := range size {
				tgt.Files[fmt.Sprintf("/dest-%d.txt", j)] = []byte("hello")
			}
			src.Files["/config.scampi"] = []byte(cfgStr)

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Symlink(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				tgt.Symlinks[fmt.Sprintf("/link-%d.txt", j)] = "/target.txt"
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Dir(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				tgt.Dirs[fmt.Sprintf("/mydir-%d", j)] = 0o755
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Mixed(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/src.txt"] = []byte("hello")
			src.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				tgt.Files[fmt.Sprintf("/dest-%d.txt", j)] = []byte("hello")
				tgt.Symlinks[fmt.Sprintf("/link-%d.txt", j)] = "/target.txt"
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Template(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				tgt.Files[fmt.Sprintf("/out-%d.conf", j)] = []byte("server bench port=8080")
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Pkg(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			tgt.Pkgs["nginx"] = true

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Service(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			tgt.Services["nginx"] = true
			tgt.EnabledServices["nginx"] = true

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Group(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			for j := range size {
				name := fmt.Sprintf("deploy-%d", j)
				tgt.Groups[name] = target.GroupInfo{Name: name}
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_User(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
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
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Sysctl(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			tgt.Files["/etc/sysctl.d/99-scampi-net-ipv4-ip_forward.conf"] = []byte("net.ipv4.ip_forward = 1\n")
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				if cmd == "sysctl -n net.ipv4.ip_forward" {
					return target.CommandResult{ExitCode: 0, Stdout: "1\n"}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Firewall(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
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
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Run(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				if cmd == "check-thing" {
					return target.CommandResult{ExitCode: 0}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Container(b *testing.B) {
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

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.scampi"] = []byte(cfgStr)
			for i := range size {
				tgt.Containers[fmt.Sprintf("app-%d", i)] = target.ContainerInfo{
					Name: fmt.Sprintf("app-%d", i), Image: "nginx:1.25",
					Running: true, Restart: "unless-stopped",
				}
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Unarchive_TarGz(b *testing.B) {
	benchUnarchiveNoOp(b, makeTarGz, "/data.tar.gz")
}

func BenchmarkApplyNoOp_Unarchive_TarXz(b *testing.B) {
	benchUnarchiveNoOp(b, makeTarXz, "/data.tar.xz")
}

func BenchmarkApplyNoOp_Unarchive_TarZst(b *testing.B) {
	benchUnarchiveNoOp(b, makeTarZst, "/data.tar.zst")
}

func BenchmarkApplyNoOp_Unarchive_Tar(b *testing.B) {
	benchUnarchiveNoOp(b, makeTar, "/data.tar")
}

func BenchmarkApplyNoOp_Unarchive_Zip(b *testing.B) {
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

			src := source.NewMemSource()
			src.Files[srcPath] = archive
			src.Files["/config.scampi"] = []byte(cfgStr)

			tgt := target.NewMemTarget()
			tgt.Files[destMarkerPath("/output")] = []byte(archiveHash(archive) + "\n")

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Mount(b *testing.B) {
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

			src := source.NewMemSource()
			src.Files["/config.scampi"] = []byte(cfgStr)

			tgt := target.NewMemTarget()
			tgt.Files["/etc/fstab"] = []byte(fstab.String())
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				// findmnt --target <dest> --noheadings → exit 0 means mounted.
				if strings.HasPrefix(cmd, "findmnt --target ") {
					rest := strings.TrimPrefix(cmd, "findmnt --target ")
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
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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

func BenchmarkApplyNoOp_Runset(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			// Each run_set step lists exactly one item, and desired
			// matches it — no add, no remove, no drift.
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

			src := source.NewMemSource()
			src.Files["/config.scampi"] = []byte(cfgStr)

			tgt := target.NewMemTarget()
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				// list-N returns "item-N\n" so the desired set matches.
				if strings.HasPrefix(cmd, "list-") {
					n := strings.TrimPrefix(cmd, "list-")
					return target.CommandResult{ExitCode: 0, Stdout: "item-" + n + "\n"}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			cmdsDone := recordCmdsOp(b, tgt)
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			cmdsDone()
		})
	}
}

// Benchmark: Apply() no-op run for rest.request (idempotent path)
// -----------------------------------------------------------------------------
//
// Drift detection on REST resources follows the rest.request idempotency
// check path: a GET against the resource returns a status that the
// rest.status check accepts. No POST/DELETE is needed in steady state.
// MemREST counts as "calls"; we report it as http/op alongside cmds/op.

func BenchmarkApplyNoOp_REST(b *testing.B) {
	sizes := benchSizes(1, 10, 100, 1000)
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			var cfgEntries strings.Builder
			routes := make(map[string]target.MemRESTResponse, size)
			for i := range size {
				path := fmt.Sprintf("/items/%d", i)
				fmt.Fprintf(
					&cfgEntries,
					"  rest.request { method = \"GET\", path = %q, check = rest.status { code = 200 } }\n",
					path,
				)
				routes["GET "+path] = target.MemRESTResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)}
			}

			cfgStr := fmt.Sprintf(`module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = "http://localhost:8080" }

std.deploy(name = "bench", targets = [api]) {
%s}
`, cfgEntries.String())

			src := source.NewMemSource()
			src.Files["/config.scampi"] = []byte(cfgStr)

			tgt := target.NewMemREST(routes)

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			startCalls := len(tgt.Calls())
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
			if b.N > 0 {
				delta := len(tgt.Calls()) - startCalls
				b.ReportMetric(float64(delta)/float64(b.N), "http/op")
			}
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

func BenchmarkApplyMixed_Cold(b *testing.B) {
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

			src := source.NewMemSource()
			src.Files["/config.scampi"] = []byte(cfgStr)

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			var cmdTotal int
			for b.Loop() {
				b.StopTimer()
				tgt := target.NewMemTarget()
				b.StartTimer()

				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = harness.MockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if _, err = e.Apply(ctx); err != nil {
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
