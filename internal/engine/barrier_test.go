// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"testing"

	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/copy"
	"scampi.dev/scampi/internal/step/pkg"
	"scampi.dev/scampi/internal/step/run"
	"scampi.dev/scampi/internal/step/service"
)

// Opaque-step barrier guarantee
// -----------------------------------------------------------------------------
//
// posix.run, posix.pkg, and posix.service emit steps whose side
// effects scampi cannot statically reason about: a `run` is arbitrary
// shell, a `pkg` install can drop files anywhere on disk, a `service`
// start may rewrite `/etc` via post-start units. Every one of these
// has to fence — concurrent execution against any sibling step is
// unsafe.
//
// The mechanism today: such steps don't implement spec.Promiser
// (or implement it trivially) and `hasResources` reports false, which
// makes `buildStepGraph` chain them as barriers. These tests pin
// that contract: regressing the barrier is silent in the engine —
// nothing crashes, you just race.

// asStep plans a step instance (StepID is irrelevant for graph
// shape — the engine only cares about Type/Config) and returns the
// resulting step.
func asStep(t *testing.T, st spec.StepKind, cfg any) spec.Step {
	t.Helper()
	act, err := st.Plan(spec.DeclaredStep{Type: st, Config: cfg})
	if err != nil {
		t.Fatalf("%s.Plan: %v", st.Kind(), err)
	}
	return act
}

func TestRunStepIsBarrier(t *testing.T) {
	act := asStep(t, run.Run{}, &run.RunConfig{
		Apply: "echo hi",
		Check: "true",
	})
	if hasResources(act) {
		// posix.run is arbitrary shell — concurrent execution alongside
		// any other step is unsafe.
		t.Fatal("posix.run step must be a barrier")
	}
}

func TestPkgStepIsBarrier(t *testing.T) {
	act := asStep(t, pkg.Pkg{}, &pkg.PkgConfig{
		Packages: []string{"vim"},
		Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
		State:    "present",
	})
	if hasResources(act) {
		// pkg installs can drop files anywhere and run arbitrary
		// post-install hooks.
		t.Fatal("posix.pkg step must be a barrier")
	}
}

func TestServiceStepIsBarrier(t *testing.T) {
	act := asStep(t, service.Service{}, &service.ServiceConfig{
		Name:    "samba-ad-dc",
		State:   "running",
		Enabled: true,
	})
	if hasResources(act) {
		// service start hooks can rewrite /etc, spawn child units, etc.
		t.Fatal("posix.service step must be a barrier")
	}
}

func TestSerialDeployBlockOrders_PkgServiceRun(t *testing.T) {
	// dc1-v2-shaped sequence: pkg → service → run → run → run → service.
	// Every step is opaque (barrier), so the fence builder must chain
	// them strictly: each step depends on the immediately preceding
	// one. If anyone adds Promiser to one of these step types without
	// an exact, complete declaration, this test fails.
	steps := []spec.Step{
		asStep(t, pkg.Pkg{}, &pkg.PkgConfig{
			Packages: []string{"samba"},
			Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
			State:    "present",
		}),
		asStep(t, service.Service{}, &service.ServiceConfig{
			Name: "smbd", State: "stopped", Enabled: false,
		}),
		asStep(t, run.Run{}, &run.RunConfig{
			Apply: "samba-tool domain provision ...", Check: "test -f /var/lib/samba/private/sam.ldb",
		}),
		asStep(t, run.Run{}, &run.RunConfig{
			Apply: "install -m 0644 /var/lib/samba/private/krb5.conf /etc/krb5.conf",
			Check: "cmp -s /var/lib/samba/private/krb5.conf /etc/krb5.conf",
		}),
		asStep(t, service.Service{}, &service.ServiceConfig{
			Name: "samba-ad-dc", State: "running", Enabled: true,
		}),
	}
	nodes := buildStepGraph(steps)

	// Every node except the first depends on the previous one; the
	// chain is linear because every step is a barrier.
	for i := 1; i < len(nodes); i++ {
		requiresExactly(t, nodes[i], nodes[i].step.Kind(), nodes[i-1])
	}
}

func TestBarrierFencesAcrossPatherSteps(t *testing.T) {
	// posix.copy declares a path resource (it's a Pather, NOT a barrier).
	// A run between two copies must still fence — the run can read or
	// write anything, including files copy is touching.
	steps := []spec.Step{
		asStep(t, copy.Copy{}, &copy.CopyConfig{
			Dest:  "/etc/krb5.conf",
			Perm:  "0644",
			Owner: "root",
			Group: "root",
		}),
		asStep(t, run.Run{}, &run.RunConfig{
			Apply: "samba-tool ...", Check: "test -f /var/lib/samba/private/sam.ldb",
		}),
		asStep(t, copy.Copy{}, &copy.CopyConfig{
			Dest:  "/etc/samba/smb.conf",
			Perm:  "0644",
			Owner: "root",
			Group: "root",
		}),
	}
	nodes := buildStepGraph(steps)

	// run (idx 1) must depend on the prior copy.
	requiresExactly(t, nodes[1], "run", nodes[0])
	// the second copy (idx 2) must depend on the run barrier.
	requiresExactly(t, nodes[2], "copy", nodes[1])
}
