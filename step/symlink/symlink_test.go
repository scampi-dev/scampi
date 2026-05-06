// SPDX-License-Identifier: GPL-3.0-only

package symlink

import (
	"path/filepath"
	"testing"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

// Overwrite-existing-file behavior (#279)
// -----------------------------------------------------------------------------
// posix.symlink used to refuse a non-symlink at the link path, forcing
// users into `posix.run { ln -sf ... }`. These tests pin the new
// contract: any non-matching state at the link path is drift to be
// reconciled by remove + symlink. Refusing to act would force the
// `posix.run` workaround back into every config that wants to replace
// a package-shipped config with a managed one.

func newOp(linkTarget, link string) *ensureSymlinkOp {
	return &ensureSymlinkOp{
		BaseOp: sharedop.BaseOp{},
		target: linkTarget,
		link:   link,
	}
}

func setupParentDir(tgt *target.MemTarget, dir string) {
	// Lstat on the parent dir is part of Check; an explicit dir entry
	// satisfies it without having to seed an unrelated file.
	tgt.Dirs[dir] = 0o755
}

func TestEnsureSymlink_OverwritesRegularFile(t *testing.T) {
	tgt := target.NewMemTarget()
	setupParentDir(tgt, "/etc")
	tgt.Files["/etc/krb5.conf"] = []byte("# Debian's stock krb5-config drop\n")
	tgt.Modes["/etc/krb5.conf"] = 0o644

	op := newOp("/var/lib/samba/private/krb5.conf", "/etc/krb5.conf")

	// Check: regular file is drift, not an error.
	res, drift, err := op.Check(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if res != spec.CheckUnsatisfied {
		t.Errorf("Check = %v, want CheckUnsatisfied", res)
	}
	if len(drift) != 1 || drift[0].Current != "regular file" || drift[0].Desired != "/var/lib/samba/private/krb5.conf" {
		t.Errorf("drift = %+v, want one entry showing current=regular file, desired=<target>", drift)
	}

	// Execute: file replaced with symlink.
	result, err := op.Execute(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Changed {
		t.Error("Execute should report Changed=true")
	}
	if _, stillExists := tgt.Files["/etc/krb5.conf"]; stillExists {
		t.Error("regular file at link path should have been removed")
	}
	if got := tgt.Symlinks["/etc/krb5.conf"]; got != "/var/lib/samba/private/krb5.conf" {
		t.Errorf("symlink target = %q, want /var/lib/samba/private/krb5.conf", got)
	}
}

func TestEnsureSymlink_IdempotentOnExistingCorrectLink(t *testing.T) {
	tgt := target.NewMemTarget()
	setupParentDir(tgt, "/etc")
	tgt.Symlinks["/etc/krb5.conf"] = "/var/lib/samba/private/krb5.conf"

	op := newOp("/var/lib/samba/private/krb5.conf", "/etc/krb5.conf")

	res, drift, err := op.Check(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if res != spec.CheckSatisfied {
		t.Errorf("Check = %v (drift=%+v), want CheckSatisfied", res, drift)
	}

	result, err := op.Execute(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Changed {
		t.Error("Execute on already-correct symlink should report Changed=false")
	}
}

func TestEnsureSymlink_OverwritesWrongTargetSymlink(t *testing.T) {
	tgt := target.NewMemTarget()
	setupParentDir(tgt, "/etc")
	tgt.Symlinks["/etc/krb5.conf"] = "/some/other/path"

	op := newOp("/var/lib/samba/private/krb5.conf", "/etc/krb5.conf")

	res, drift, err := op.Check(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if res != spec.CheckUnsatisfied {
		t.Errorf("Check = %v, want CheckUnsatisfied", res)
	}
	if len(drift) != 1 || drift[0].Current != "/some/other/path" {
		t.Errorf("drift = %+v, want current=/some/other/path", drift)
	}

	if _, err := op.Execute(t.Context(), nil, tgt); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := tgt.Symlinks["/etc/krb5.conf"]; got != "/var/lib/samba/private/krb5.conf" {
		t.Errorf("symlink retargeted to %q, want /var/lib/samba/private/krb5.conf", got)
	}
}

func TestEnsureSymlink_DriftReportsDirectory(t *testing.T) {
	tgt := target.NewMemTarget()
	setupParentDir(tgt, "/etc")
	// MemTarget reports a path as a directory when at least one entry
	// lives below it (implicit dir semantics). Seeding a child file
	// is the canonical way to materialize a directory entry.
	tgt.Files["/etc/krb5.conf/child"] = []byte{}
	tgt.Modes["/etc/krb5.conf/child"] = 0o644

	op := newOp("/var/lib/samba/private/krb5.conf", "/etc/krb5.conf")

	res, drift, err := op.Check(t.Context(), nil, tgt)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if res != spec.CheckUnsatisfied {
		t.Errorf("Check = %v, want CheckUnsatisfied", res)
	}
	if len(drift) != 1 || drift[0].Current != "directory" {
		t.Errorf("drift = %+v, want current=directory", drift)
	}
}

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name   string
		target string
		link   string
		want   string
	}{
		{
			name:   "absolute target unchanged",
			target: "/absolute/path/to/target",
			link:   "/some/link",
			want:   "/absolute/path/to/target",
		},
		{
			name:   "absolute target with absolute link",
			target: "/path/to/target.txt",
			link:   "/path/to/link.txt",
			want:   "/path/to/target.txt",
		},
		{
			name:   "relative target same directory",
			target: "./dir/target.txt",
			link:   "./dir/link.txt",
			want:   "target.txt",
		},
		{
			name:   "relative target parent directory",
			target: "./target.txt",
			link:   "./subdir/link.txt",
			want:   filepath.Join("..", "target.txt"),
		},
		{
			name:   "relative target sibling directory",
			target: "./other/target.txt",
			link:   "./subdir/link.txt",
			want:   filepath.Join("..", "other", "target.txt"),
		},
		{
			name:   "deeply nested relative paths",
			target: "./a/b/c/target.txt",
			link:   "./x/y/z/link.txt",
			want:   filepath.Join("..", "..", "..", "a", "b", "c", "target.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTarget(tt.target, tt.link)
			if err != nil {
				t.Fatalf("resolveTarget() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}
