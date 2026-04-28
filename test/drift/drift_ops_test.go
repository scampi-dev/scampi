// SPDX-License-Identifier: GPL-3.0-only

package drift

import (
	"context"
	"io/fs"
	"testing"

	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/copy"
	"scampi.dev/scampi/step/pkg"
	"scampi.dev/scampi/step/sharedops/fileops"
	stepsymlink "scampi.dev/scampi/step/symlink"
	"scampi.dev/scampi/step/sysctl"
	"scampi.dev/scampi/step/template"
	"scampi.dev/scampi/target"
)

func planOps(
	t *testing.T,
	stepType spec.StepType,
	cfg any,
	fields map[string]spec.FieldSpan,
) []spec.Op {
	t.Helper()
	step := spec.StepInstance{
		Type:   stepType,
		Config: cfg,
		Fields: fields,
	}
	action, err := stepType.Plan(step)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	return action.Ops()
}

type checker interface {
	Check(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error)
}

func mustCheckDrift(
	t *testing.T,
	op checker,
	src source.Source,
	tgt target.Target,
) []spec.DriftDetail {
	t.Helper()
	_, drift, err := op.Check(context.Background(), src, tgt)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return drift
}

func collectDrift(
	t *testing.T,
	ops []spec.Op,
	src source.Source,
	tgt target.Target,
) []spec.DriftDetail {
	t.Helper()
	var all []spec.DriftDetail
	for _, op := range ops {
		_, drift, err := op.Check(context.Background(), src, tgt)
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		all = append(all, drift...)
	}
	return all
}

func assertDrift(
	t *testing.T,
	got []spec.DriftDetail,
	field, current, desired string,
) {
	t.Helper()
	for _, d := range got {
		if d.Field == field {
			if d.Current != current {
				t.Errorf(
					"field %q: current = %q, want %q",
					field, d.Current, current,
				)
			}
			if d.Desired != desired {
				t.Errorf(
					"field %q: desired = %q, want %q",
					field, d.Desired, desired,
				)
			}
			return
		}
	}
	t.Errorf("field %q not found in drift details: %+v", field, got)
}

// copyFileOp
// -----------------------------------------------------------------------------

func TestDrift_CopyFile_Missing(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/src.txt"] = []byte("hello")
	tgt := target.NewMemTarget()
	// dest file doesn't exist

	ops := planOps(t, copy.Copy{}, &copy.CopyConfig{
		Src: spec.SourceRef{Kind: spec.SourceLocal, Path: "/src.txt"}, Dest: "/dest.txt", Perm: "0644",
	}, map[string]spec.FieldSpan{
		"src":  {},
		"dest": {},
		"perm": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "content", "", "5 bytes")
}

func TestDrift_CopyFile_ContentDiffers(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/src.txt"] = []byte("new content")
	tgt := target.NewMemTarget()
	tgt.Files["/dest.txt"] = []byte("old")
	tgt.Modes["/dest.txt"] = 0o644

	ops := planOps(t, copy.Copy{}, &copy.CopyConfig{
		Src: spec.SourceRef{Kind: spec.SourceLocal, Path: "/src.txt"}, Dest: "/dest.txt", Perm: "0644",
	}, map[string]spec.FieldSpan{
		"src":  {},
		"dest": {},
		"perm": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "content", "3 bytes", "11 bytes")
}

// renderTemplateOp
// -----------------------------------------------------------------------------

func TestDrift_RenderTemplate_Missing(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/tmpl.txt"] = []byte("Hello {{.Name}}")
	tgt := target.NewMemTarget()

	ops := planOps(t, template.Template{}, &template.TemplateConfig{
		Src:  spec.SourceRef{Kind: spec.SourceLocal, Path: "/tmpl.txt"},
		Dest: "/out.txt",
		Perm: "0644",
		Data: template.DataConfig{
			Values: map[string]any{"Name": "World"},
		},
	}, map[string]spec.FieldSpan{
		"src":  {},
		"dest": {},
		"perm": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "content", "", "11 bytes")
}

func TestDrift_RenderTemplate_ContentDiffers(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/tmpl.txt"] = []byte("Hello {{.Name}}")
	tgt := target.NewMemTarget()
	tgt.Files["/out.txt"] = []byte("old text here")
	tgt.Modes["/out.txt"] = 0o644

	ops := planOps(t, template.Template{}, &template.TemplateConfig{
		Src:  spec.SourceRef{Kind: spec.SourceLocal, Path: "/tmpl.txt"},
		Dest: "/out.txt",
		Perm: "0644",
		Data: template.DataConfig{
			Values: map[string]any{"Name": "World"},
		},
	}, map[string]spec.FieldSpan{
		"src":  {},
		"dest": {},
		"perm": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "content", "13 bytes", "11 bytes")
}

// ensureSymlinkOp
// -----------------------------------------------------------------------------

func TestDrift_Symlink_Missing(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	// Create a file so the parent dir exists implicitly
	tgt.Files["/usr/local/bin/placeholder"] = []byte{}
	tgt.Modes["/usr/local/bin/placeholder"] = 0o644

	ops := planOps(
		t, stepsymlink.Symlink{}, &stepsymlink.SymlinkConfig{
			Target: "/usr/bin/real",
			Link:   "/usr/local/bin/mylink",
		}, map[string]spec.FieldSpan{
			"target": {},
			"link":   {},
		},
	)

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "target", "", "/usr/bin/real")
}

func TestDrift_Symlink_WrongTarget(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Symlinks["/usr/local/bin/mylink"] = "/usr/bin/old"
	// Parent dir needs to exist
	tgt.Files["/usr/local/bin/placeholder"] = []byte{}
	tgt.Modes["/usr/local/bin/placeholder"] = 0o644

	ops := planOps(
		t, stepsymlink.Symlink{}, &stepsymlink.SymlinkConfig{
			Target: "/usr/bin/real",
			Link:   "/usr/local/bin/mylink",
		}, map[string]spec.FieldSpan{
			"target": {},
			"link":   {},
		},
	)

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "target", "/usr/bin/old", "/usr/bin/real")
}

// EnsureModeOp
// -----------------------------------------------------------------------------

func TestDrift_EnsureMode_Missing(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	op := &fileops.EnsureModeOp{Path: "/etc/app.conf", Mode: 0o644}
	details := mustCheckDrift(t, op, src, tgt)
	assertDrift(t, details, "perm", "", "-rw-r--r--")
}

func TestDrift_EnsureMode_Differs(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Files["/etc/app.conf"] = []byte("content")
	tgt.Modes["/etc/app.conf"] = 0o755

	op := &fileops.EnsureModeOp{Path: "/etc/app.conf", Mode: 0o644}
	details := mustCheckDrift(t, op, src, tgt)
	assertDrift(
		t,
		details,
		"perm",
		fs.FileMode(0o755).String(),
		fs.FileMode(0o644).String(),
	)
}

// EnsureOwnerOp
// -----------------------------------------------------------------------------

func TestDrift_EnsureOwner_Missing(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	op := &fileops.EnsureOwnerOp{
		Path: "/etc/app.conf", Owner: "app", Group: "staff",
	}
	details := mustCheckDrift(t, op, src, tgt)
	assertDrift(t, details, "owner:group", "", "app:staff")
}

func TestDrift_EnsureOwner_Differs(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Files["/etc/app.conf"] = []byte("content")
	tgt.Modes["/etc/app.conf"] = 0o644
	tgt.Owners["/etc/app.conf"] = target.Owner{
		User: "root", Group: "wheel",
	}

	op := &fileops.EnsureOwnerOp{
		Path: "/etc/app.conf", Owner: "app", Group: "staff",
	}
	details := mustCheckDrift(t, op, src, tgt)
	assertDrift(t, details, "owner:group", "root:wheel", "app:staff")
}

// ensurePkgOp
// -----------------------------------------------------------------------------

func TestDrift_Pkg_NotInstalled(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	// vim not installed

	ops := planOps(t, pkg.Pkg{}, &pkg.PkgConfig{
		Packages: []string{"vim"},
		State:    "present",
		Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
	}, map[string]spec.FieldSpan{
		"packages": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "vim: not installed", "present")
}

func TestDrift_Pkg_Upgradable(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Pkgs["vim"] = true
	tgt.Upgradable["vim"] = true

	ops := planOps(t, pkg.Pkg{}, &pkg.PkgConfig{
		Packages: []string{"vim"},
		State:    "latest",
		Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
	}, map[string]spec.FieldSpan{
		"packages": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "vim: upgradable", "latest")
}

// TestDrift_Pkg_Latest_CheckIsReadOnly is the regression for #242:
// state=latest must not refresh the package cache during Check.
// Cache refresh is a target mutation and belongs in Execute.
func TestDrift_Pkg_Latest_CheckIsReadOnly(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Pkgs["vim"] = true
	tgt.CacheStale = true // simulate cache that would otherwise be refreshed

	ops := planOps(t, pkg.Pkg{}, &pkg.PkgConfig{
		Packages: []string{"vim"},
		State:    "latest",
		Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
	}, map[string]spec.FieldSpan{
		"packages": {},
	})

	_ = collectDrift(t, ops, src, tgt)

	if !tgt.CacheStale {
		t.Error("Check refreshed the pkg cache (CacheStale -> false) — Check must be read-only")
	}
}

func TestDrift_Pkg_WantAbsent(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Pkgs["vim"] = true

	ops := planOps(t, pkg.Pkg{}, &pkg.PkgConfig{
		Packages: []string{"vim"},
		State:    "absent",
		Source:   spec.PkgSourceRef{Kind: spec.PkgSourceNative},
	}, map[string]spec.FieldSpan{
		"packages": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "vim: installed", "absent")
}

// setSysctlOp
// -----------------------------------------------------------------------------

func sysctlMemTarget(cmdFunc func(string) (target.CommandResult, error)) *target.MemTarget {
	tgt := target.NewMemTarget()
	tgt.CommandFunc = cmdFunc
	return tgt
}

func TestDrift_Sysctl_ValueDiffers(t *testing.T) {
	src := source.NewMemSource()
	tgt := sysctlMemTarget(func(cmd string) (target.CommandResult, error) {
		if cmd == "sysctl -n net.ipv4.ip_forward" {
			return target.CommandResult{Stdout: "0\n"}, nil
		}
		return target.CommandResult{ExitCode: 127}, nil
	})

	ops := planOps(t, sysctl.Sysctl{}, &sysctl.SysctlConfig{
		Key: "net.ipv4.ip_forward", Value: "1", Persist: false,
	}, map[string]spec.FieldSpan{
		"key":   {},
		"value": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "net.ipv4.ip_forward", "0", "1")
}

func TestDrift_Sysctl_AlreadySatisfied(t *testing.T) {
	src := source.NewMemSource()
	tgt := sysctlMemTarget(func(cmd string) (target.CommandResult, error) {
		if cmd == "sysctl -n net.ipv4.ip_forward" {
			return target.CommandResult{Stdout: "1\n"}, nil
		}
		return target.CommandResult{ExitCode: 127}, nil
	})

	ops := planOps(t, sysctl.Sysctl{}, &sysctl.SysctlConfig{
		Key: "net.ipv4.ip_forward", Value: "1", Persist: false,
	}, map[string]spec.FieldSpan{
		"key":   {},
		"value": {},
	})

	details := collectDrift(t, ops, src, tgt)
	if len(details) != 0 {
		t.Errorf("expected no drift, got %+v", details)
	}
}

// persistSysctlOp
// -----------------------------------------------------------------------------

func TestDrift_Sysctl_DropInMissing(t *testing.T) {
	src := source.NewMemSource()
	tgt := sysctlMemTarget(func(cmd string) (target.CommandResult, error) {
		if cmd == "sysctl -n net.ipv4.ip_forward" {
			return target.CommandResult{Stdout: "1\n"}, nil
		}
		return target.CommandResult{ExitCode: 127}, nil
	})

	ops := planOps(t, sysctl.Sysctl{}, &sysctl.SysctlConfig{
		Key: "net.ipv4.ip_forward", Value: "1", Persist: true,
	}, map[string]spec.FieldSpan{
		"key":   {},
		"value": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(
		t,
		details,
		"drop-in",
		"(absent)",
		"/etc/sysctl.d/99-scampi-net-ipv4-ip_forward.conf",
	)
}

func TestDrift_Sysctl_DropInContentDiffers(t *testing.T) {
	src := source.NewMemSource()
	tgt := sysctlMemTarget(func(cmd string) (target.CommandResult, error) {
		if cmd == "sysctl -n net.ipv4.ip_forward" {
			return target.CommandResult{Stdout: "1\n"}, nil
		}
		return target.CommandResult{ExitCode: 127}, nil
	})
	tgt.Files["/etc/sysctl.d/99-scampi-net-ipv4-ip_forward.conf"] = []byte("net.ipv4.ip_forward = 0\n")

	ops := planOps(t, sysctl.Sysctl{}, &sysctl.SysctlConfig{
		Key: "net.ipv4.ip_forward", Value: "1", Persist: true,
	}, map[string]spec.FieldSpan{
		"key":   {},
		"value": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(
		t,
		details,
		"drop-in",
		"net.ipv4.ip_forward = 0\n",
		"net.ipv4.ip_forward = 1\n",
	)
}

// cleanupSysctlOp
// -----------------------------------------------------------------------------

func TestDrift_Sysctl_CleanupStaleDropIn(t *testing.T) {
	src := source.NewMemSource()
	tgt := sysctlMemTarget(func(cmd string) (target.CommandResult, error) {
		if cmd == "sysctl -n net.ipv4.ip_forward" {
			return target.CommandResult{Stdout: "1\n"}, nil
		}
		return target.CommandResult{ExitCode: 127}, nil
	})
	tgt.Files["/etc/sysctl.d/99-scampi-net-ipv4-ip_forward.conf"] = []byte("net.ipv4.ip_forward = 1\n")

	ops := planOps(t, sysctl.Sysctl{}, &sysctl.SysctlConfig{
		Key: "net.ipv4.ip_forward", Value: "1", Persist: false,
	}, map[string]spec.FieldSpan{
		"key":   {},
		"value": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(
		t,
		details,
		"drop-in",
		"/etc/sysctl.d/99-scampi-net-ipv4-ip_forward.conf",
		"(absent)",
	)
}

func TestDrift_Sysctl_CleanupNoDropIn(t *testing.T) {
	src := source.NewMemSource()
	tgt := sysctlMemTarget(func(cmd string) (target.CommandResult, error) {
		if cmd == "sysctl -n net.ipv4.ip_forward" {
			return target.CommandResult{Stdout: "1\n"}, nil
		}
		return target.CommandResult{ExitCode: 127}, nil
	})

	ops := planOps(t, sysctl.Sysctl{}, &sysctl.SysctlConfig{
		Key: "net.ipv4.ip_forward", Value: "1", Persist: false,
	}, map[string]spec.FieldSpan{
		"key":   {},
		"value": {},
	})

	details := collectDrift(t, ops, src, tgt)
	if len(details) != 0 {
		t.Errorf("expected no drift, got %+v", details)
	}
}
