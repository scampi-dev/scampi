// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
)

func FuzzDiagnostics(f *testing.F) {
	// ---- Seeds: real, high-value starting points ----

	seeds := []string{
		// minimal valid config
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.dir { path = "/tmp/fuzz" }
}`,

		// valid config with a copy step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.copy { src = posix.source_local { path = "/a" }, dest = "/b", perm = "0644", owner = "u", group = "g" }
}`,

		// missing required copy fields
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.copy { src = posix.source_local { path = "a" }, dest = "b" }
}`,

		// missing required symlink fields
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.symlink { target = "a" }
}`,

		// missing required template fields
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.template { src = posix.source_local { path = "a" }, dest = "b" }
}`,

		// run step with check/apply
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.run { apply = "do-thing", check = "check-thing" }
}`,

		// run step with always
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.run { apply = "do-thing", always = true }
}`,

		// run step missing check and always
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.run { apply = "do-thing" }
}`,

		// run step with conflicting check and always
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.run { apply = "do-thing", check = "check-thing", always = true }
}`,

		// run step missing apply
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.run { check = "check-thing" }
}`,

		// pkg step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.pkg { packages = ["nginx"], state = posix.PkgState.present, source = posix.pkg_system {} }
}`,

		// pkg step with invalid state
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.pkg { packages = ["nginx"], state = "bogus", source = posix.pkg_system {} }
}`,

		// service step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.service { name = "nginx", state = posix.ServiceState.running, enabled = true }
}`,

		// service step missing required fields
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.service { state = posix.ServiceState.running }
}`,

		// group step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.group { name = "deploy" }
}`,

		// group step absent
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.group { name = "oldgroup", state = posix.GroupState.absent }
}`,

		// user step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.user { name = "deploy", shell = "/bin/bash", groups = ["sudo"] }
}`,

		// user step absent
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.user { name = "olduser", state = posix.UserState.absent }
}`,

		// sysctl step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.sysctl { key = "net.ipv4.ip_forward", value = "1" }
}`,

		// sysctl step with persist=false
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.sysctl { key = "vm.swappiness", value = "10", persist = false }
}`,

		// firewall step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.firewall { port = "22/tcp", action = posix.FirewallAction.allow }
}`,

		// firewall step deny
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.firewall { port = "443/tcp", action = posix.FirewallAction.deny }
}`,

		// firewall step invalid port
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.firewall { port = "not-a-port", action = posix.FirewallAction.allow }
}`,

		// mount step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount { src = "10.10.2.2:/data", dest = "/mnt/data", fs_type = posix.MountType.nfs }
}`,

		// mount step with options
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount { src = "//server/share", dest = "/mnt/share", fs_type = posix.MountType.cifs, opts = "credentials=/etc/smbcreds,uid=1000" }
}`,

		// mount step absent
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount { src = "10.10.2.2:/data", dest = "/mnt/data", fs_type = posix.MountType.nfs, state = posix.MountState.absent }
}`,

		// mount step invalid state
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount { src = "10.10.2.2:/data", dest = "/mnt/data", fs_type = posix.MountType.nfs, state = "bogus" }
}`,

		// firewall step invalid action
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.firewall { port = "22/tcp", action = "bogus" }
}`,

		// unarchive step
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.unarchive { src = posix.source_local { path = "/data.tar.gz" }, dest = "/output" }
}`,

		// unarchive step missing required fields
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.unarchive { src = posix.source_local { path = "/data.tar.gz" } }
}`,

		// container.instance step
		`module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.25" }
}`,

		// container.instance step with all options
		`module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance {
    name = "app"
    image = "nginx:1.25"
    state = container.State.running
    restart = container.Restart.always
    ports = ["8080:80"]
    env = {"FOO": "bar"}
    mounts = ["/host:/container:ro"]
    args = ["--flag"]
    labels = {"app": "test"}
    healthcheck = container.Healthcheck { cmd = "curl -f http://localhost/" }
  }
}`,

		// container.instance stopped
		`module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", state = container.State.absent }
}`,

		// container.instance missing name
		`module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { image = "nginx:1.25" }
}`,

		// container.instance invalid state
		`module main
import "std"
import "std/posix"
import "std/container"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  container.instance { name = "app", image = "nginx:1.25", state = "bogus" }
}`,

		// syntax error: unclosed brace
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local"
std.deploy(name = "test", targets = [local]) {}`,

		// syntax error: unclosed bracket
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }
std.deploy(name = "test", targets = [local]) { posix.dir { path = [} }`,

		// type error: wrong argument type for targets
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }
std.deploy(name = "test", targets = "local") {}`,

		// unknown function call
		`module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }
frobnicate(name = "test")`,

		// garbage
		`this is not valid starlark at all @@@ !!!`,

		// empty
		``,
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		src := source.NewMemSource()
		tgt := target.NewMemTarget()

		src.Files["/config.scampi"] = []byte(input)

		rec := &recordingDisplayer{}
		em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
		store := diagnostic.NewSourceStore()

		// ---- Hard invariant: user input must not panic ----
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC on user input:\n%q\npanic: (%T) %v", input, r, r)
			}
		}()

		apply := func() error {
			ctx := context.Background()
			cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
			if err != nil {
				return err
			}

			resolved, err := engine.Resolve(cfg, "", "")
			if err != nil {
				return err
			}

			resolved.Target = mockTargetInstance(tgt)

			e, err := engine.New(ctx, src, resolved, em)
			if err != nil {
				return err
			}
			defer e.Close()

			return e.Apply(ctx)
		}

		err := apply()
		// All errors are acceptable — the no-panic invariant above
		// is what matters. Lang pipeline errors, engine errors, and
		// diagnostic errors are all valid outcomes for fuzz input.
		_ = err

		// ---- Diagnostic invariants ----
		assertEngineDiagnosticsWellFormed(t, rec.engineDiagnostics)
		assertPlanDiagnosticsWellFormed(t, rec.planDiagnostics)
		assertActionDiagnosticsWellFormed(t, rec.actionDiagnostics)
		assertOpDiagnosticsWellFormed(t, rec.opDiagnostics)
	})
}

func assertEngineDiagnosticsWellFormed(t *testing.T, diags []event.EngineDiagnostic) {
	t.Helper()
	for i, d := range diags {
		if d.Severity.String() == "" {
			t.Fatalf("engine diagnostic [%d] has empty Severity", i)
		}
		if d.Detail.Template.ID == "" {
			t.Fatalf("engine diagnostic [%d] has empty Template.ID", i)
		}
	}
}

func assertPlanDiagnosticsWellFormed(t *testing.T, diags []event.PlanDiagnostic) {
	t.Helper()
	for i, d := range diags {
		if d.Severity.String() == "" {
			t.Fatalf("plan diagnostic [%d] has empty Severity", i)
		}
		if d.Detail.Template.ID == "" {
			t.Fatalf("plan diagnostic [%d] has empty Template.ID", i)
		}
	}
}

func assertActionDiagnosticsWellFormed(t *testing.T, diags []event.ActionDiagnostic) {
	t.Helper()
	for i, d := range diags {
		if d.Severity.String() == "" {
			t.Fatalf("action diagnostic [%d] has empty Severity", i)
		}
		if d.Detail.Template.ID == "" {
			t.Fatalf("action diagnostic [%d] has empty Template.ID", i)
		}
	}
}

func assertOpDiagnosticsWellFormed(t *testing.T, diags []event.OpDiagnostic) {
	t.Helper()
	for i, d := range diags {
		if d.Severity.String() == "" {
			t.Fatalf("op diagnostic [%d] has empty Severity", i)
		}
		if d.Detail.Template.ID == "" {
			t.Fatalf("op diagnostic [%d] has empty Template.ID", i)
		}
	}
}
