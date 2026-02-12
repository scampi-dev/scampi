package test

import (
	"context"
	"io/fs"
	"testing"

	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/copy"
	"godoit.dev/doit/step/pkg"
	"godoit.dev/doit/step/sharedops/fileops"
	stepsymlink "godoit.dev/doit/step/symlink"
	"godoit.dev/doit/step/template"
	"godoit.dev/doit/target"
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
	action, err := stepType.Plan(0, step)
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
		Src: "/src.txt", Dest: "/dest.txt", Perm: "0644",
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
		Src: "/src.txt", Dest: "/dest.txt", Perm: "0644",
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
		Src:  "/tmpl.txt",
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
		Src:  "/tmpl.txt",
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
		t, details,
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
	}, map[string]spec.FieldSpan{
		"packages": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "vim: upgradable", "latest")
}

func TestDrift_Pkg_WantAbsent(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Pkgs["vim"] = true

	ops := planOps(t, pkg.Pkg{}, &pkg.PkgConfig{
		Packages: []string{"vim"},
		State:    "absent",
	}, map[string]spec.FieldSpan{
		"packages": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "vim: installed", "absent")
}
