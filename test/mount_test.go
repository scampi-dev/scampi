// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"strings"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
)

func mountCommandFunc(mounts map[string]bool) func(string) (target.CommandResult, error) {
	return func(cmd string) (target.CommandResult, error) {
		switch {
		case strings.HasPrefix(cmd, "which "):
			return target.CommandResult{ExitCode: 0}, nil
		case strings.HasPrefix(cmd, "findmnt --target "):
			dest := strings.TrimPrefix(cmd, "findmnt --target ")
			dest = strings.TrimSuffix(dest, " --noheadings")
			if mounts[dest] {
				return target.CommandResult{ExitCode: 0, Stdout: dest}, nil
			}
			return target.CommandResult{ExitCode: 1}, nil
		case strings.HasPrefix(cmd, "mount "):
			dest := strings.TrimPrefix(cmd, "mount ")
			mounts[dest] = true
			return target.CommandResult{ExitCode: 0}, nil
		case strings.HasPrefix(cmd, "umount "):
			dest := strings.TrimPrefix(cmd, "umount ")
			delete(mounts, dest)
			return target.CommandResult{ExitCode: 0}, nil
		default:
			return target.CommandResult{ExitCode: 127}, nil
		}
	}
}

func TestMount_CreateAndMount(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount { src = "10.10.2.2:/data", dest = "/mnt/data", fs_type = posix.MountType.nfs }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	mounts := map[string]bool{}
	tgt.CommandFunc = mountCommandFunc(mounts)
	tgt.Files["/etc/fstab"] = []byte("# system fstab\n")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	fstab := string(tgt.Files["/etc/fstab"])
	if !strings.Contains(fstab, "10.10.2.2:/data /mnt/data nfs defaults 0 0") {
		t.Errorf("fstab missing mount entry, got:\n%s", fstab)
	}
	if !mounts["/mnt/data"] {
		t.Error("mount point should be mounted")
	}
}

func TestMount_Idempotent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount { src = "10.10.2.2:/data", dest = "/mnt/data", fs_type = posix.MountType.nfs }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	mounts := map[string]bool{"/mnt/data": true}
	tgt.CommandFunc = mountCommandFunc(mounts)
	tgt.Files["/etc/fstab"] = []byte("# system fstab\n10.10.2.2:/data /mnt/data nfs defaults 0 0\n")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v\n%s", err, rec)
	}
}

func TestMount_DriftRemount(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount { src = "10.10.2.2:/data", dest = "/mnt/data", fs_type = posix.MountType.nfs }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	mounts := map[string]bool{}
	tgt.CommandFunc = mountCommandFunc(mounts)
	tgt.Files["/etc/fstab"] = []byte("# system fstab\n10.10.2.2:/data /mnt/data nfs defaults 0 0\n")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	if !mounts["/mnt/data"] {
		t.Error("should have mounted /mnt/data")
	}
}

func TestMount_Absent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount {
    src = "10.10.2.2:/data"
    dest = "/mnt/data"
    fs_type = posix.MountType.nfs
    state = posix.MountState.absent
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	mounts := map[string]bool{"/mnt/data": true}
	tgt.CommandFunc = mountCommandFunc(mounts)
	tgt.Files["/etc/fstab"] = []byte("# system fstab\n10.10.2.2:/data /mnt/data nfs defaults 0 0\n")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	fstab := string(tgt.Files["/etc/fstab"])
	if strings.Contains(fstab, "/mnt/data") {
		t.Errorf("fstab should not contain mount entry, got:\n%s", fstab)
	}
	if mounts["/mnt/data"] {
		t.Error("mount point should be unmounted")
	}
}

func TestMount_Unmounted(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount {
    src = "10.10.2.2:/data"
    dest = "/mnt/data"
    fs_type = posix.MountType.nfs
    state = posix.MountState.unmounted
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	mounts := map[string]bool{"/mnt/data": true}
	tgt.CommandFunc = mountCommandFunc(mounts)
	tgt.Files["/etc/fstab"] = []byte("# system fstab\n")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	fstab := string(tgt.Files["/etc/fstab"])
	if !strings.Contains(fstab, "10.10.2.2:/data /mnt/data nfs defaults 0 0") {
		t.Errorf("fstab should contain mount entry, got:\n%s", fstab)
	}
	if mounts["/mnt/data"] {
		t.Error("mount point should NOT be mounted")
	}
}

func TestMount_OptsChange(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount {
    src = "10.10.2.2:/data"
    dest = "/mnt/data"
    fs_type = posix.MountType.nfs
    opts = "defaults,noatime"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	mounts := map[string]bool{"/mnt/data": true}
	tgt.CommandFunc = mountCommandFunc(mounts)
	tgt.Files["/etc/fstab"] = []byte("10.10.2.2:/data /mnt/data nfs defaults 0 0\n")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	fstab := string(tgt.Files["/etc/fstab"])
	if !strings.Contains(fstab, "defaults,noatime") {
		t.Errorf("fstab should have updated opts, got:\n%s", fstab)
	}
}

func TestMount_AbsentAlreadyGone(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"

let local = posix.local { name = "local" }

std.deploy(name = "test", targets = [local]) {
  posix.mount {
    src = "10.10.2.2:/data"
    dest = "/mnt/data"
    fs_type = posix.MountType.nfs
    state = posix.MountState.absent
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	mounts := map[string]bool{}
	tgt.CommandFunc = mountCommandFunc(mounts)
	tgt.Files["/etc/fstab"] = []byte("# empty\n")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v\n%s", err, rec)
	}
}
