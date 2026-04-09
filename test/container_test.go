// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"testing"
	"time"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
)

func TestContainer_CreateAndRun(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.25" }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	info, ok := tgt.Containers["app"]
	if !ok {
		t.Fatal("container not created")
	}
	if !info.Running {
		t.Error("container should be running")
	}
	if info.Image != "nginx:1.25" {
		t.Errorf("image: got %q, want %q", info.Image, "nginx:1.25")
	}
	if info.Restart != "unless-stopped" {
		t.Errorf("restart: got %q, want %q", info.Restart, "unless-stopped")
	}
}

func TestContainer_Idempotent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.25" }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
	}
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

	if !tgt.Containers["app"].Running {
		t.Error("container should still be running")
	}
	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions on idempotent run")
		}
	}
}

func TestContainer_ImageDrift_Recreates(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.26" }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
	}
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

	info := tgt.Containers["app"]
	if info.Image != "nginx:1.26" {
		t.Errorf("image: got %q, want %q", info.Image, "nginx:1.26")
	}
	if !info.Running {
		t.Error("container should be running after recreate")
	}
}

func TestContainer_WithLabels(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    labels = {"app": "myapp", "env": "prod"}
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	info, ok := tgt.Containers["app"]
	if !ok {
		t.Fatal("container not created")
	}
	if info.Labels["app"] != "myapp" {
		t.Errorf("label app: got %q, want %q", info.Labels["app"], "myapp")
	}
	if info.Labels["env"] != "prod" {
		t.Errorf("label env: got %q, want %q", info.Labels["env"], "prod")
	}
}

func TestContainer_LabelsIdempotent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    labels = {"app": "myapp"}
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Labels:  map[string]string{"app": "myapp", "org.opencontainers.image.title": "nginx"},
	}
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

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions on idempotent run")
		}
	}
}

func TestContainer_LabelDrift_Recreates(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    labels = {"env": "staging"}
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Labels:  map[string]string{"env": "prod"},
	}
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

	info := tgt.Containers["app"]
	if info.Labels["env"] != "staging" {
		t.Errorf("label env: got %q, want %q", info.Labels["env"], "staging")
	}
	if !info.Running {
		t.Error("container should be running after recreate")
	}
}

func TestContainer_PortWithIPAndProto(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    ports = ["127.0.0.1:9090:80", "3000:3000/udp"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	info, ok := tgt.Containers["app"]
	if !ok {
		t.Fatal("container not created")
	}

	wantBound := target.Port{
		HostIP: "127.0.0.1", HostPort: "9090", ContainerPort: "80",
	}
	wantUDP := target.Port{
		HostPort: "3000", ContainerPort: "3000", Proto: target.ProtoUDP,
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

func TestContainer_WithArgs(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    args = ["--verbose", "--debug"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	info, ok := tgt.Containers["app"]
	if !ok {
		t.Fatal("container not created")
	}
	if len(info.Args) != 2 || info.Args[0] != "--verbose" || info.Args[1] != "--debug" {
		t.Errorf("args: got %v, want [--verbose --debug]", info.Args)
	}
}

func TestContainer_ArgsIdempotent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    args = ["--verbose"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Args:    []string{"--verbose"},
	}
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

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions on idempotent run")
		}
	}
}

func TestContainer_ArgsDrift_Recreates(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    args = ["--new-flag"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Args:    []string{"--old-flag"},
	}
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

	info := tgt.Containers["app"]
	if len(info.Args) != 1 || info.Args[0] != "--new-flag" {
		t.Errorf("args: got %v, want [--new-flag]", info.Args)
	}
	if !info.Running {
		t.Error("container should be running after recreate")
	}
}

func TestContainer_NoArgsDeclared_ImageDefaultIgnored(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.25" }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Args:    []string{"--some-image-default"},
	}
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

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions — image default args should not cause drift")
		}
	}
}

func TestContainer_WithHealthcheck(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    healthcheck = container.Healthcheck {
      cmd = "curl -f http://localhost/"
      interval = "10s"
      timeout = "5s"
      retries = 5
    }
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	info, ok := tgt.Containers["app"]
	if !ok {
		t.Fatal("container not created")
	}
	if !info.Running {
		t.Error("container should be running")
	}
	if info.Healthcheck == nil {
		t.Fatal("healthcheck should be set")
	}
	if info.Healthcheck.Cmd != "curl -f http://localhost/" {
		t.Errorf("healthcheck cmd: got %q", info.Healthcheck.Cmd)
	}
	if info.Healthcheck.Interval != 10*time.Second {
		t.Errorf("healthcheck interval: got %v", info.Healthcheck.Interval)
	}
	if info.Healthcheck.Timeout != 5*time.Second {
		t.Errorf("healthcheck timeout: got %v", info.Healthcheck.Timeout)
	}
	if info.Healthcheck.Retries != 5 {
		t.Errorf("healthcheck retries: got %d", info.Healthcheck.Retries)
	}
	if info.HealthStatus != "healthy" {
		t.Errorf("health status: got %q, want %q", info.HealthStatus, "healthy")
	}
}

func TestContainer_HealthcheckDefaults(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    healthcheck = container.Healthcheck { cmd = "true" }
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	info := tgt.Containers["app"]
	hc := info.Healthcheck
	if hc == nil {
		t.Fatal("healthcheck should be set")
	}
	if hc.Interval != 30*time.Second {
		t.Errorf("default interval: got %v, want 30s", hc.Interval)
	}
	if hc.Timeout != 30*time.Second {
		t.Errorf("default timeout: got %v, want 30s", hc.Timeout)
	}
	if hc.Retries != 3 {
		t.Errorf("default retries: got %d, want 3", hc.Retries)
	}
}

func TestContainer_HealthcheckDrift_Recreates(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    healthcheck = container.Healthcheck { cmd = "curl -f http://localhost/new" }
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Healthcheck: &target.Healthcheck{
			Cmd:      "curl -f http://localhost/old",
			Interval: 30 * time.Second,
			Timeout:  30 * time.Second,
			Retries:  3,
		},
		HealthStatus: "healthy",
	}
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

	info := tgt.Containers["app"]
	if info.Healthcheck.Cmd != "curl -f http://localhost/new" {
		t.Errorf("cmd: got %q", info.Healthcheck.Cmd)
	}
}

func TestContainer_NoHealthcheckDeclared_ImageDefaultIgnored(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.25" }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Healthcheck: &target.Healthcheck{
			Cmd:      "curl -f http://localhost/",
			Interval: 30 * time.Second,
			Timeout:  30 * time.Second,
			Retries:  3,
		},
		HealthStatus: "healthy",
	}
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

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions — image healthcheck should not cause drift")
		}
	}
}

func TestContainer_Stopped(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.25", state = container.State.stopped }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
	}
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

	info := tgt.Containers["app"]
	if info.Running {
		t.Error("container should be stopped")
	}
}

func TestContainer_Absent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", state = container.State.absent }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
	}
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

	if _, ok := tgt.Containers["app"]; ok {
		t.Error("container should have been removed")
	}
}

func TestContainer_Absent_AlreadyGone(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", state = container.State.absent }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions when already absent")
		}
	}
}

func TestContainer_WithEnv(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    env = {"DB_HOST": "db.local", "DB_PORT": "5432"}
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	info, ok := tgt.Containers["app"]
	if !ok {
		t.Fatal("container not created")
	}
	if info.Env["DB_HOST"] != "db.local" {
		t.Errorf("env DB_HOST: got %q, want %q", info.Env["DB_HOST"], "db.local")
	}
	if info.Env["DB_PORT"] != "5432" {
		t.Errorf("env DB_PORT: got %q, want %q", info.Env["DB_PORT"], "5432")
	}
}

func TestContainer_EnvIdempotent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    env = {"DB_HOST": "db.local"}
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Env:     map[string]string{"DB_HOST": "db.local", "PATH": "/usr/bin"},
	}
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

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions on idempotent run")
		}
	}
}

func TestContainer_EnvDrift_Recreates(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    env = {"DB_HOST": "db.new"}
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Env:     map[string]string{"DB_HOST": "db.old"},
	}
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

	info := tgt.Containers["app"]
	if info.Env["DB_HOST"] != "db.new" {
		t.Errorf("env DB_HOST: got %q, want %q", info.Env["DB_HOST"], "db.new")
	}
	if !info.Running {
		t.Error("container should be running after recreate")
	}
}

func TestContainer_WithMounts(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    mounts = ["/opt/data:/data", "/opt/config:/config:ro"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Dirs["/opt/data"] = 0o755
	tgt.Dirs["/opt/config"] = 0o755
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

	info, ok := tgt.Containers["app"]
	if !ok {
		t.Fatal("container not created")
	}
	if len(info.Mounts) != 2 {
		t.Fatalf("mounts: got %d, want 2", len(info.Mounts))
	}
}

func TestContainer_MountIdempotent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    mounts = ["/opt/data:/data"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Mounts:  []target.Mount{{Source: "/opt/data", Target: "/data"}},
	}
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

	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("expected no op executions on idempotent run")
		}
	}
}

func TestContainer_MountDrift_Recreates(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    mounts = ["/opt/new:/data"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Dirs["/opt/new"] = 0o755
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Mounts:  []target.Mount{{Source: "/opt/old", Target: "/data"}},
	}
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

	info := tgt.Containers["app"]
	wantMount := target.Mount{Source: "/opt/new", Target: "/data"}
	if len(info.Mounts) != 1 || info.Mounts[0] != wantMount {
		t.Errorf("mounts: got %v, want [%s]", info.Mounts, wantMount)
	}
	if !info.Running {
		t.Error("container should be running after recreate")
	}
}

func TestContainer_MountSourceMissing_Aborts(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    mounts = ["/nonexistent:/data"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected abort when mount source does not exist")
	}
}

func TestContainer_MountSourcePromised_Deferred(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.dir { path = "/opt/data" }
  container.instance {
    name = "app"
    image = "nginx:1.25"
    mounts = ["/opt/data:/data"]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
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

	info, ok := tgt.Containers["app"]
	if !ok {
		t.Fatal("container not created")
	}
	if !info.Running {
		t.Error("container should be running")
	}
}

func TestContainer_PortDrift_Recreates(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.25", ports = ["9090:80"] }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["app"] = target.ContainerInfo{
		Name: "app", Image: "nginx:1.25", Running: true,
		Restart: "unless-stopped",
		Ports:   []target.Port{{HostPort: "8080", ContainerPort: "80"}},
	}
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

	info := tgt.Containers["app"]
	wantPort := target.Port{HostPort: "9090", ContainerPort: "80"}
	if len(info.Ports) != 1 || info.Ports[0] != wantPort {
		t.Errorf("ports: got %v, want [9090:80]", info.Ports)
	}
	if !info.Running {
		t.Error("container should be running after recreate")
	}
}
