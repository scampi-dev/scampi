// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
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
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

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
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for dir (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Dir(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        dir(
            desc="dir-%%d" %% i,
            path="/mydir",
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Dirs["/mydir"] = 0o755

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

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
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for template (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Template(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        template(
            desc="tmpl-%%d" %% i,
            content="server {{ .name }} port={{ .port }}",
            dest="/out.conf",
            perm="0644",
            owner="perf-owner",
            group="perf-group",
            data={"values": {"name": "bench", "port": 8080}},
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Files["/out.conf"] = []byte("server bench port=8080")

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for pkg (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Pkg(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        pkg(
            desc="pkg-%%d" %% i,
            packages=["nginx"],
            state="present",
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Pkgs["nginx"] = true

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for service (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Service(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        service(
            desc="svc-%%d" %% i,
            name="nginx",
            state="running",
            enabled=True,
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Services["nginx"] = true
			tgt.EnabledServices["nginx"] = true

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for group (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Group(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        group(
            desc="grp-%%d" %% i,
            name="deploy",
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Groups["deploy"] = target.GroupInfo{Name: "deploy"}

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for user (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_User(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        user(
            desc="usr-%%d" %% i,
            name="deploy",
            shell="/bin/bash",
            groups=["sudo"],
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Users["deploy"] = target.UserInfo{
				Name:   "deploy",
				Shell:  "/bin/bash",
				Groups: []string{"sudo"},
			}

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for sysctl (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Sysctl(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        sysctl(
            desc="sysctl-%%d" %% i,
            key="net.ipv4.ip_forward",
            value="1",
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.Files["/etc/sysctl.d/99-scampi-net-ipv4-ip_forward.conf"] = []byte("net.ipv4.ip_forward = 1\n")
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				if cmd == "sysctl -n net.ipv4.ip_forward" {
					return target.CommandResult{ExitCode: 0, Stdout: "1\n"}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for firewall (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Firewall(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        firewall(
            desc="fw-%%d" %% i,
            port="22/tcp",
            action="allow",
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
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

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})
	}
}

// Benchmark: Apply() no-op run for run step (idempotent path)
// -----------------------------------------------------------------------------

func BenchmarkApplyNoOp_Run(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(
    name="bench",
    targets=["local"],
    steps=[
        run(
            desc="run-%%d" %% i,
            apply="do-thing",
            check="check-thing",
        )
        for i in range(%d)
    ],
)
`, size)

			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			src.Files["/config.star"] = []byte(cfgStr)
			tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
				if cmd == "check-thing" {
					return target.CommandResult{ExitCode: 0}, nil
				}
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
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

var benchArchiveSizes = []int{1, 10, 100, 1000}

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
	for _, n := range benchArchiveSizes {
		archive := makeFn(b, generateFiles(n))
		b.Run(fmt.Sprintf("Size-%d", n), func(b *testing.B) {
			cfgStr := fmt.Sprintf(`target.local(name="local")

deploy(name="bench", targets=["local"], steps=[
    unarchive(src="%s", dest="/output", depth=0),
])
`, srcPath)

			src := source.NewMemSource()
			src.Files[srcPath] = archive
			src.Files["/config.star"] = []byte(cfgStr)

			tgt := target.NewMemTarget()
			tgt.Files[destMarkerPath("/output")] = []byte(archiveHash(archive) + "\n")

			rec := &recordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := spec.NewSourceStore()

			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
				if err != nil {
					b.Fatalf("engine.LoadConfig() must not return error, got %v", err)
				}

				resolved, err := engine.Resolve(cfg, "", "")
				if err != nil {
					b.Fatalf("engine.Resolve() must not return error, got %v", err)
				}

				resolved.Target = mockTargetInstance(tgt)

				e, err := engine.New(ctx, src, resolved, em)
				if err != nil {
					b.Fatalf("engine.New() must not return error, got %v", err)
				}

				if err := e.Apply(ctx); err != nil {
					b.Fatal(err)
				}
				e.Close()
				cancel()
			}
		})

	}
}
