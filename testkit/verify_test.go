// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"strings"
	"testing"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/target"
)

// expectState builds an ExpectedState StructVal with the given
// per-slot map of (key → matcher StructVal). Slots with nil maps
// are omitted entirely (None).
func expectState(slots map[string]map[string]*eval.StructVal) *eval.StructVal {
	sv := &eval.StructVal{
		TypeName: "ExpectedState",
		QualName: "test.ExpectedState",
		RetType:  "ExpectedState",
		Fields:   make(map[string]eval.Value),
	}
	for slot, entries := range slots {
		mp := &eval.MapVal{}
		for k, v := range entries {
			mp.Keys = append(mp.Keys, &eval.StringVal{V: k})
			mp.Values = append(mp.Values, v)
		}
		sv.Fields[slot] = mp
	}
	return sv
}

func TestVerifyMemTarget_FilesPass(t *testing.T) {
	mock := target.NewMemTarget()
	mock.Files["/etc/foo"] = []byte("hello world")

	expect := expectState(map[string]map[string]*eval.StructVal{
		"files": {
			"/etc/foo": matcher("has_substring", map[string]string{"substring": "world"}),
		},
	})

	if got := VerifyMemTarget(expect, mock); len(got) != 0 {
		t.Errorf("expected no mismatches, got: %+v", got)
	}
}

func TestVerifyMemTarget_FilesFail(t *testing.T) {
	mock := target.NewMemTarget()
	mock.Files["/etc/foo"] = []byte("hello")

	expect := expectState(map[string]map[string]*eval.StructVal{
		"files": {
			"/etc/foo": matcher("has_exact_content", map[string]string{"content": "goodbye"}),
		},
	})

	got := VerifyMemTarget(expect, mock)
	if len(got) != 1 {
		t.Fatalf("expected 1 mismatch, got %d: %+v", len(got), got)
	}
	if got[0].Slot != SlotFileContent || got[0].Key != "/etc/foo" {
		t.Errorf("wrong slot/key: %+v", got[0])
	}
	if !strings.Contains(got[0].Reason, "exact content mismatch") {
		t.Errorf("wrong reason: %q", got[0].Reason)
	}
}

func TestVerifyMemTarget_FileAbsence(t *testing.T) {
	mock := target.NewMemTarget()
	// /banned is intentionally not seeded.

	expect := expectState(map[string]map[string]*eval.StructVal{
		"files": {
			"/banned": matcher("is_absent", nil),
		},
	})

	if got := VerifyMemTarget(expect, mock); len(got) != 0 {
		t.Errorf("expected no mismatches, got: %+v", got)
	}
}

func TestVerifyMemTarget_Packages(t *testing.T) {
	mock := target.NewMemTarget()
	mock.Pkgs["nginx"] = true

	expect := expectState(map[string]map[string]*eval.StructVal{
		"packages": {
			"nginx":   matcher("has_pkg_status", map[string]string{"status": "present"}),
			"apache2": matcher("has_pkg_status", map[string]string{"status": "absent"}),
		},
	})

	if got := VerifyMemTarget(expect, mock); len(got) != 0 {
		t.Errorf("expected no mismatches, got: %+v", got)
	}
}

func TestVerifyMemTarget_Services(t *testing.T) {
	mock := target.NewMemTarget()
	mock.Services["nginx"] = true
	mock.Services["redis"] = false

	expect := expectState(map[string]map[string]*eval.StructVal{
		"services": {
			"nginx": matcher("has_svc_status", map[string]string{"status": "running"}),
			"redis": matcher("has_svc_status", map[string]string{"status": "stopped"}),
		},
	})

	if got := VerifyMemTarget(expect, mock); len(got) != 0 {
		t.Errorf("expected no mismatches, got: %+v", got)
	}
}

func TestVerifyMemTarget_Dirs(t *testing.T) {
	mock := target.NewMemTarget()
	mock.Dirs["/var/log/myapp"] = 0o755

	expect := expectState(map[string]map[string]*eval.StructVal{
		"dirs": {
			"/var/log/myapp": matcher("is_present", nil),
			"/missing":       matcher("is_absent", nil),
		},
	})

	if got := VerifyMemTarget(expect, mock); len(got) != 0 {
		t.Errorf("expected no mismatches, got: %+v", got)
	}
}

func TestVerifyMemTarget_Symlinks(t *testing.T) {
	mock := target.NewMemTarget()
	mock.Symlinks["/usr/local/bin/foo"] = "/opt/foo/bin/foo"

	expect := expectState(map[string]map[string]*eval.StructVal{
		"symlinks": {
			"/usr/local/bin/foo": matcher("is_present", nil),
		},
	})

	if got := VerifyMemTarget(expect, mock); len(got) != 0 {
		t.Errorf("expected no mismatches, got: %+v", got)
	}
}

func TestVerifyMemTarget_StableOrder(t *testing.T) {
	mock := target.NewMemTarget()
	// Three intentional failures, one per slot, with map keys
	// chosen so the per-slot iteration order would be unstable
	// without sorting.
	mock.Files["/z/a.conf"] = []byte("a")
	mock.Files["/a/z.conf"] = []byte("z")
	mock.Pkgs["zlib"] = true

	expect := expectState(map[string]map[string]*eval.StructVal{
		"files": {
			"/z/a.conf": matcher("has_exact_content", map[string]string{"content": "WRONG"}),
			"/a/z.conf": matcher("has_exact_content", map[string]string{"content": "WRONG"}),
		},
		"packages": {
			"zlib": matcher("has_pkg_status", map[string]string{"status": "absent"}),
		},
	})

	got := VerifyMemTarget(expect, mock)
	if len(got) != 3 {
		t.Fatalf("expected 3 mismatches, got %d", len(got))
	}
	// Files come before packages (slot ordering), and within files
	// keys are sorted alphabetically.
	if got[0].Key != "/a/z.conf" || got[0].Slot != SlotFileContent {
		t.Errorf("[0] = %+v", got[0])
	}
	if got[1].Key != "/z/a.conf" || got[1].Slot != SlotFileContent {
		t.Errorf("[1] = %+v", got[1])
	}
	if got[2].Key != "zlib" || got[2].Slot != SlotPackageStatus {
		t.Errorf("[2] = %+v", got[2])
	}
}

func TestVerifyMemTarget_NilInputs(t *testing.T) {
	if got := VerifyMemTarget(nil, target.NewMemTarget()); got != nil {
		t.Errorf("nil expect should be no-op, got %+v", got)
	}
	expect := expectState(map[string]map[string]*eval.StructVal{
		"files": {"/x": matcher("is_present", nil)},
	})
	if got := VerifyMemTarget(expect, nil); got != nil {
		t.Errorf("nil mock should be no-op, got %+v", got)
	}
}

func TestVerifyMemTarget_OmittedSlot(t *testing.T) {
	mock := target.NewMemTarget()
	expect := expectState(nil) // no slots set
	if got := VerifyMemTarget(expect, mock); got != nil {
		t.Errorf("empty expect should produce no mismatches, got %+v", got)
	}
}
