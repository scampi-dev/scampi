// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"testing"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/copy"
	"scampi.dev/scampi/step/pkg"
	"scampi.dev/scampi/step/run"
	"scampi.dev/scampi/step/service"
)

// Opaque-action barrier guarantee
// -----------------------------------------------------------------------------
//
// posix.run, posix.pkg, and posix.service emit actions whose side
// effects scampi cannot statically reason about: a `run` is arbitrary
// shell, a `pkg` install can drop files anywhere on disk, a `service`
// start may rewrite `/etc` via post-start units. Every one of these
// has to fence — concurrent execution against any sibling step is
// unsafe.
//
// The mechanism today: such actions don't implement spec.Promiser
// (or implement it trivially) and `hasResources` reports false, which
// makes `buildActionGraph` chain them as barriers. These tests pin
// that contract: regressing the barrier is silent in the engine —
// nothing crashes, you just race.

// asAction plans a step instance (StepID is irrelevant for graph
// shape — the engine only cares about Type/Config) and returns the
// resulting action.
func asAction(t *testing.T, st spec.StepType, cfg any) spec.Action {
	t.Helper()
	act, err := st.Plan(spec.StepInstance{Type: st, Config: cfg})
	if err != nil {
		t.Fatalf("%s.Plan: %v", st.Kind(), err)
	}
	return act
}

func TestRunActionIsBarrier(t *testing.T) {
	act := asAction(t, run.Run{}, &run.RunConfig{
		Apply: "echo hi",
		Check: "true",
	})
	if hasResources(act) {
		// posix.run is arbitrary shell — concurrent execution alongside
		// any other action is unsafe.
		t.Fatal("posix.run action must be a barrier")
	}
}

func TestPkgActionIsBarrier(t *testing.T) {
	act := asAction(t, pkg.Pkg{}, &pkg.PkgConfig{
		Packages: []string{"vim"},
		Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
		State:    "present",
	})
	if hasResources(act) {
		// pkg installs can drop files anywhere and run arbitrary
		// post-install hooks.
		t.Fatal("posix.pkg action must be a barrier")
	}
}

func TestServiceActionIsBarrier(t *testing.T) {
	act := asAction(t, service.Service{}, &service.ServiceConfig{
		Name:    "samba-ad-dc",
		State:   "running",
		Enabled: true,
	})
	if hasResources(act) {
		// service start hooks can rewrite /etc, spawn child units, etc.
		t.Fatal("posix.service action must be a barrier")
	}
}

func TestSerialDeployBlockOrders_PkgServiceRun(t *testing.T) {
	// dc1-v2-shaped sequence: pkg → service → run → run → run → service.
	// Every action is opaque (barrier), so the fence builder must chain
	// them strictly: each action depends on the immediately preceding
	// one. If anyone adds Promiser to one of these step types without
	// an exact, complete declaration, this test fails.
	steps := []spec.Action{
		asAction(t, pkg.Pkg{}, &pkg.PkgConfig{
			Packages: []string{"samba"},
			Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
			State:    "present",
		}),
		asAction(t, service.Service{}, &service.ServiceConfig{
			Name: "smbd", State: "stopped", Enabled: false,
		}),
		asAction(t, run.Run{}, &run.RunConfig{
			Apply: "samba-tool domain provision ...", Check: "test -f /var/lib/samba/private/sam.ldb",
		}),
		asAction(t, run.Run{}, &run.RunConfig{
			Apply: "install -m 0644 /var/lib/samba/private/krb5.conf /etc/krb5.conf",
			Check: "cmp -s /var/lib/samba/private/krb5.conf /etc/krb5.conf",
		}),
		asAction(t, service.Service{}, &service.ServiceConfig{
			Name: "samba-ad-dc", State: "running", Enabled: true,
		}),
	}
	nodes := buildActionGraph(steps)

	// Every node except the first depends on the previous one; the
	// chain is linear because every action is a barrier.
	for i := 1; i < len(nodes); i++ {
		requiresExactly(t, nodes[i], nodes[i].action.Kind(), nodes[i-1])
	}
}

func TestBarrierFencesAcrossPatherActions(t *testing.T) {
	// posix.copy declares a path resource (it's a Pather, NOT a barrier).
	// A run between two copies must still fence — the run can read or
	// write anything, including files copy is touching.
	steps := []spec.Action{
		asAction(t, copy.Copy{}, &copy.CopyConfig{
			Dest:  "/etc/krb5.conf",
			Perm:  "0644",
			Owner: "root",
			Group: "root",
		}),
		asAction(t, run.Run{}, &run.RunConfig{
			Apply: "samba-tool ...", Check: "test -f /var/lib/samba/private/sam.ldb",
		}),
		asAction(t, copy.Copy{}, &copy.CopyConfig{
			Dest:  "/etc/samba/smb.conf",
			Perm:  "0644",
			Owner: "root",
			Group: "root",
		}),
	}
	nodes := buildActionGraph(steps)

	// run (idx 1) must depend on the prior copy.
	requiresExactly(t, nodes[1], "run", nodes[0])
	// the second copy (idx 2) must depend on the run barrier.
	requiresExactly(t, nodes[2], "copy", nodes[1])
}
