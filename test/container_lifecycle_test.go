// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/local"
)

// setupContainerTest creates a local target with container support and
// registers cleanup for the named container. Skips if no container runtime
// is available.
func setupContainerTest(t *testing.T, name string) target.Target {
	t.Helper()

	if os.Getenv("SCAMPI_TEST_CONTAINERS") == "" {
		t.Skip("container tests disabled (set SCAMPI_TEST_CONTAINERS=1)")
	}

	ctx := context.Background()
	tgt, err := local.Local{}.Create(ctx, source.NewMemSource(), spec.TargetInstance{})
	if err != nil {
		t.Fatalf("create local target: %v", err)
	}

	cm, ok := tgt.(target.ContainerManager)
	if !ok {
		t.Skip("local target does not implement ContainerManager")
	}

	if !tgt.Capabilities().HasAll(capability.Container) {
		t.Skip("no container runtime detected")
	}

	t.Cleanup(func() {
		_ = cm.StopContainer(context.Background(), name)
		_ = cm.RemoveContainer(context.Background(), name)
	})

	return tgt
}

func containerName(t *testing.T) string {
	t.Helper()
	// Derive a unique, docker-safe name from the test name.
	name := strings.ToLower(t.Name())
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")
	return "scampi-test-" + name
}

func applyContainerConfig(t *testing.T, cfgStr string, tgt target.Target) *recordingDisplayer {
	t.Helper()
	src := source.NewMemSource()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}
	return rec
}

func TestContainerLifecycle_CreateAndRun(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "%s", image = "traefik/whoami" }
}
`, name)

	applyContainerConfig(t, cfgStr, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, err := cm.InspectContainer(context.Background(), name)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !exists {
		t.Fatal("container should exist")
	}
	if !info.Running {
		t.Error("container should be running")
	}
	if info.Image != "traefik/whoami" {
		t.Errorf("image: got %q, want %q", info.Image, "traefik/whoami")
	}
}

func TestContainerLifecycle_Idempotent(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "%s", image = "traefik/whoami" }
}
`, name)

	// First run: create
	applyContainerConfig(t, cfgStr, tgt)

	// Second run: should be idempotent
	rec := applyContainerConfig(t, cfgStr, tgt)
	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions on idempotent run")
		}
	}

	cm := tgt.(target.ContainerManager)
	info, exists, _ := cm.InspectContainer(context.Background(), name)
	if !exists || !info.Running {
		t.Error("container should still be running")
	}
}

func TestContainerLifecycle_WithEnv(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "traefik/whoami"
    env = {"MY_VAR": "hello", "MY_OTHER": "world"}
  }
}
`, name)

	applyContainerConfig(t, cfgStr, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, err := cm.InspectContainer(context.Background(), name)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !exists {
		t.Fatal("container should exist")
	}
	if info.Env["MY_VAR"] != "hello" {
		t.Errorf("env MY_VAR: got %q, want %q", info.Env["MY_VAR"], "hello")
	}
	if info.Env["MY_OTHER"] != "world" {
		t.Errorf("env MY_OTHER: got %q, want %q", info.Env["MY_OTHER"], "world")
	}
}

func TestContainerLifecycle_EnvDrift(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	// Create with initial env
	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "traefik/whoami"
    env = {"MY_VAR": "old_value"}
  }
}
`, name)
	applyContainerConfig(t, cfgStr, tgt)

	// Update env — should trigger recreate
	cfgStr2 := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "traefik/whoami"
    env = {"MY_VAR": "new_value"}
  }
}
`, name)
	applyContainerConfig(t, cfgStr2, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, _ := cm.InspectContainer(context.Background(), name)
	if !exists {
		t.Fatal("container should exist after drift recreate")
	}
	if info.Env["MY_VAR"] != "new_value" {
		t.Errorf("env MY_VAR: got %q, want %q", info.Env["MY_VAR"], "new_value")
	}
	if !info.Running {
		t.Error("container should be running after recreate")
	}
}

func TestContainerLifecycle_Ports(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "traefik/whoami"
    ports = ["18080:80"]
  }
}
`, name)

	applyContainerConfig(t, cfgStr, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, _ := cm.InspectContainer(context.Background(), name)
	if !exists {
		t.Fatal("container should exist")
	}
	wantPort := target.Port{HostPort: "18080", ContainerPort: "80"}
	found := false
	for _, p := range info.Ports {
		if p == wantPort {
			found = true
		}
	}
	if !found {
		t.Errorf("ports: got %v, want to contain %s", info.Ports, wantPort)
	}
}

func TestContainerLifecycle_PortIPAndProto(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "traefik/whoami"
    ports = ["127.0.0.1:18091:80", "18092:80/udp"]
  }
}
`, name)

	applyContainerConfig(t, cfgStr, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, err := cm.InspectContainer(context.Background(), name)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !exists {
		t.Fatal("container should exist")
	}

	wantBound := target.Port{
		HostIP: "127.0.0.1", HostPort: "18091", ContainerPort: "80",
		Proto: target.ProtoTCP,
	}
	wantUDP := target.Port{
		HostPort: "18092", ContainerPort: "80", Proto: target.ProtoUDP,
	}

	foundBound, foundUDP := false, false
	for _, p := range info.Ports {
		if p == wantBound {
			foundBound = true
		}
		if p == wantUDP {
			foundUDP = true
		}
	}
	if !foundBound {
		t.Errorf("missing bound port %s in %v", wantBound, info.Ports)
	}
	if !foundUDP {
		t.Errorf("missing UDP port %s in %v", wantUDP, info.Ports)
	}
}

func TestContainerLifecycle_Labels(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "traefik/whoami"
    labels = {"app": "myapp", "env": "test"}
  }
}
`, name)

	applyContainerConfig(t, cfgStr, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, err := cm.InspectContainer(context.Background(), name)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !exists {
		t.Fatal("container should exist")
	}
	if info.Labels["app"] != "myapp" {
		t.Errorf("label app: got %q, want %q", info.Labels["app"], "myapp")
	}
	if info.Labels["env"] != "test" {
		t.Errorf("label env: got %q, want %q", info.Labels["env"], "test")
	}
}

func TestContainerLifecycle_Args(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "traefik/whoami"
    args = ["--verbose", "--port", "8080"]
  }
}
`, name)

	applyContainerConfig(t, cfgStr, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, err := cm.InspectContainer(context.Background(), name)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !exists {
		t.Fatal("container should exist")
	}
	want := []string{"--verbose", "--port", "8080"}
	if len(info.Args) != len(want) {
		t.Fatalf("args: got %v, want %v", info.Args, want)
	}
	for i, a := range want {
		if info.Args[i] != a {
			t.Errorf("args[%d]: got %q, want %q", i, info.Args[i], a)
		}
	}
}

func TestContainerLifecycle_Mounts(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	mountDir := t.TempDir()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "traefik/whoami"
    mounts = ["%s:/data:ro"]
  }
}
`, name, mountDir)

	applyContainerConfig(t, cfgStr, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, err := cm.InspectContainer(context.Background(), name)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !exists {
		t.Fatal("container should exist")
	}
	wantMount := target.Mount{Source: mountDir, Target: "/data", ReadOnly: true}
	found := false
	for _, m := range info.Mounts {
		if m == wantMount {
			found = true
		}
	}
	if !found {
		t.Errorf("mounts: got %v, want to contain %s", info.Mounts, wantMount)
	}
}

func TestContainerLifecycle_Healthcheck(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "%s"
    image = "nginx:alpine"
    healthcheck = container.Healthcheck {
      cmd = "wget -qO- http://localhost/ || exit 1"
      interval = "2s"
      timeout = "2s"
      retries = 3
    }
  }
}
`, name)

	applyContainerConfig(t, cfgStr, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, err := cm.InspectContainer(context.Background(), name)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !exists {
		t.Fatal("container should exist")
	}
	if !info.Running {
		t.Error("container should be running")
	}
	if info.HealthStatus != "healthy" {
		t.Errorf("health status: got %q, want %q", info.HealthStatus, "healthy")
	}
	if info.Healthcheck == nil {
		t.Fatal("healthcheck config should be set")
	}
	if info.Healthcheck.Cmd != "wget -qO- http://localhost/ || exit 1" {
		t.Errorf("healthcheck cmd: got %q", info.Healthcheck.Cmd)
	}
}

func TestContainerLifecycle_Stopped(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	// First create and run
	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "%s", image = "traefik/whoami" }
}
`, name)
	applyContainerConfig(t, cfgStr, tgt)

	// Now declare stopped
	cfgStr2 := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "%s", image = "traefik/whoami", state = container.State.stopped }
}
`, name)
	applyContainerConfig(t, cfgStr2, tgt)

	cm := tgt.(target.ContainerManager)
	info, exists, _ := cm.InspectContainer(context.Background(), name)
	if !exists {
		t.Fatal("container should exist when stopped")
	}
	if info.Running {
		t.Error("container should be stopped")
	}
}

func TestContainerLifecycle_Absent(t *testing.T) {
	name := containerName(t)
	tgt := setupContainerTest(t, name)

	// First create and run
	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "%s", image = "traefik/whoami" }
}
`, name)
	applyContainerConfig(t, cfgStr, tgt)

	// Now declare absent
	cfgStr2 := fmt.Sprintf(`
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "%s", state = container.State.absent }
}
`, name)
	applyContainerConfig(t, cfgStr2, tgt)

	cm := tgt.(target.ContainerManager)
	_, exists, _ := cm.InspectContainer(context.Background(), name)
	if exists {
		t.Error("container should not exist after absent")
	}
}
