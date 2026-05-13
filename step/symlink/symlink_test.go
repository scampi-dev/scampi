// SPDX-License-Identifier: GPL-3.0-only

package symlink

import (
	"path/filepath"
	"testing"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

// Apply-side behavioral scenarios (overwrite-regular-file, idempotent,
// retarget-wrong-link) live in test/testdata/e2e/symlink-* — they drive
// real scampi configs through engine.Apply against MemTarget. This
// file keeps only Check-only behavior (drift detail for non-applicable
// states) and pure-function unit tests.

func newOp(linkTarget, link string) *ensureSymlinkOp {
	return &ensureSymlinkOp{
		BaseOp: sharedop.BaseOp{},
		target: linkTarget,
		link:   link,
	}
}

// A directory at the link path is reported as drift on Check. Apply
// reconciliation isn't in scope (would require recursive directory
// removal); the test asserts the Check-time signal only.
func TestEnsureSymlink_DriftReportsDirectory(t *testing.T) {
	tgt := target.NewMemTarget()
	tgt.Dirs["/etc"] = 0o755
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
