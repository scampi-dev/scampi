// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"errors"
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
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    dir(path="/tmp/fuzz"),
])`,

		// valid config with a copy step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    copy(src=local("/a"), dest="/b", perm="0644", owner="u", group="g"),
])`,

		// missing required copy fields
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    copy(src=local("a"), dest="b"),
])`,

		// missing required symlink fields
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    symlink(target="a"),
])`,

		// missing required template fields
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    template(src=local("a"), dest="b"),
])`,

		// run step with check/apply
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    run(apply="do-thing", check="check-thing"),
])`,

		// run step with always
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    run(apply="do-thing", always=True),
])`,

		// run step missing check and always
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    run(apply="do-thing"),
])`,

		// run step with conflicting check and always
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    run(apply="do-thing", check="check-thing", always=True),
])`,

		// run step missing apply
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    run(check="check-thing"),
])`,

		// pkg step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    pkg(packages=["nginx"], state="present", source=system()),
])`,

		// pkg step with invalid state
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    pkg(packages=["nginx"], state="bogus", source=system()),
])`,

		// service step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    service(name="nginx", state="running", enabled=True),
])`,

		// service step missing required fields
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    service(state="running"),
])`,

		// group step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    group(name="deploy"),
])`,

		// group step absent
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    group(name="oldgroup", state="absent"),
])`,

		// user step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    user(name="deploy", shell="/bin/bash", groups=["sudo"]),
])`,

		// user step absent
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    user(name="olduser", state="absent"),
])`,

		// sysctl step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    sysctl(key="net.ipv4.ip_forward", value="1"),
])`,

		// sysctl step with persist=False
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    sysctl(key="vm.swappiness", value="10", persist=False),
])`,

		// firewall step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    firewall(port="22/tcp", action="allow"),
])`,

		// firewall step deny
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    firewall(port="443/tcp", action="deny"),
])`,

		// firewall step invalid port
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    firewall(port="not-a-port", action="allow"),
])`,

		// firewall step invalid action
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    firewall(port="22/tcp", action="bogus"),
])`,

		// unarchive step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    unarchive(src=local("/data.tar.gz"), dest="/output"),
])`,

		// unarchive step missing required fields
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    unarchive(src=local("/data.tar.gz")),
])`,

		// container.instance step
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    container.instance(name="app", image="nginx:1.25"),
])`,

		// container.instance step with all options
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    container.instance(
        name="app",
        image="nginx:1.25",
        state="running",
        restart="always",
        ports=["8080:80"],
        env={"FOO": "bar"},
        mounts=["/host:/container:ro"],
        args=["--flag"],
        labels={"app": "test"},
        healthcheck=container.healthcheck.cmd(cmd="curl -f http://localhost/"),
    ),
])`,

		// container.instance stopped
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    container.instance(name="app", state="absent"),
])`,

		// container.instance missing name
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    container.instance(image="nginx:1.25"),
])`,

		// container.instance invalid state
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[
    container.instance(name="app", image="nginx:1.25", state="bogus"),
])`,

		// syntax error: unclosed paren
		`target.local(name="local"
deploy(name="test", targets=["local"], steps=[])`,

		// syntax error: unclosed bracket
		`target.local(name="local")
deploy(name="test", targets=["local"], steps=[)`,

		// type error: wrong argument type for targets
		`target.local(name="local")
deploy(name="test", targets="local", steps=[])`,

		// unknown function call
		`target.local(name="local")
frobnicate(name="test")`,

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

		src.Files["/config.star"] = []byte(input)

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
			cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
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
		// ---- Error classification invariant ----
		// All errors should be either:
		// 1. AbortError (from engine execution)
		// 2. Diagnostic with ImpactAbort (from config loading/resolution)
		if err != nil {
			var abort engine.AbortError
			var diag diagnostic.Diagnostic
			isAbortError := errors.As(err, &abort)
			isAbortDiagnostic := errors.As(err, &diag) && diag.Impact() == diagnostic.ImpactAbort
			if !isAbortError && !isAbortDiagnostic {
				t.Fatalf("unexpected error type %T: %v", err, err)
			}
		}

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
