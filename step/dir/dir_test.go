// SPDX-License-Identifier: GPL-3.0-only

package dir

import (
	"testing"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

// Overwrite-existing-file behavior (#280). Same shape as #279
// (symlink): refusing to convert a non-directory at the path was
// imperative thinking. IaC says "this path is a directory"; anything
// else is drift to be reconciled, not a hard error.

func newOp(path string) *ensureDirOp {
	return &ensureDirOp{
		BaseOp: sharedop.BaseOp{},
		path:   path,
	}
}

func TestEnsureDir_OverwritesRegularFile(t *testing.T) {
	tgt := target.NewMemTarget()
	tgt.Files["/var/lib/myapp"] = []byte("placeholder dropped by installer")
	tgt.Modes["/var/lib/myapp"] = 0o644

	op := newOp("/var/lib/myapp")

	// Check: regular file at desired-dir path is drift, not an error.
	res, drift, err := op.Check(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if res != spec.CheckUnsatisfied {
		t.Errorf("Check = %v, want CheckUnsatisfied", res)
	}
	if len(drift) != 1 || drift[0].Current != "regular file" || drift[0].Desired != "directory" {
		t.Errorf("drift = %+v, want one entry showing current=regular file, desired=directory", drift)
	}

	// Execute: file replaced with directory.
	result, err := op.Execute(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Changed {
		t.Error("Execute should report Changed=true")
	}
	if _, stillExists := tgt.Files["/var/lib/myapp"]; stillExists {
		t.Error("regular file should have been removed")
	}
	if _, isDir := tgt.Dirs["/var/lib/myapp"]; !isDir {
		t.Error("directory should exist at the path")
	}
}

func TestEnsureDir_IdempotentOnExistingDirectory(t *testing.T) {
	tgt := target.NewMemTarget()
	// Materialize via implicit-dir semantics (a child entry).
	tgt.Files["/var/lib/myapp/child"] = []byte{}
	tgt.Modes["/var/lib/myapp/child"] = 0o644

	op := newOp("/var/lib/myapp")

	res, _, err := op.Check(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if res != spec.CheckSatisfied {
		t.Errorf("Check = %v, want CheckSatisfied", res)
	}

	result, err := op.Execute(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Changed {
		t.Error("Execute on already-existing directory should report Changed=false")
	}
}

func TestEnsureDir_CreatesWhenMissing(t *testing.T) {
	tgt := target.NewMemTarget()
	op := newOp("/var/lib/myapp")

	res, drift, err := op.Check(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if res != spec.CheckUnsatisfied {
		t.Errorf("Check = %v, want CheckUnsatisfied", res)
	}
	if len(drift) != 1 || drift[0].Desired != "directory" {
		t.Errorf("drift = %+v, want missing→directory", drift)
	}

	if _, err := op.Execute(t.Context(), nil, tgt); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if _, isDir := tgt.Dirs["/var/lib/myapp"]; !isDir {
		t.Error("directory should be created")
	}
}
