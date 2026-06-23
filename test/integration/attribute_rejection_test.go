// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

// End-to-end coverage for the link-time rejection paths added in
// #166. These tests construct minimal scampi configs that violate
// one specific stub attribute and assert that the linker rejects
// them before plan/apply runs. The white-box tests in
// linker/attribute_validation_test.go cover the StaticCheck contract
// directly; this file proves the lang → linker → attribute pipeline
// wires up correctly for every annotated step parameter.
//
// Each entry is a complete scampi config — they're tiny because the
// goal is to isolate one bad input. Source/target wiring is the same
// for every case so a single table iteration handles them all.

func TestAttributeRejection_LinkTime(t *testing.T) {
	cases := []struct {
		name string
		cfg  string
	}{
		// copy
		{
			name: "copy_dest_relative",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.copy {
    src = posix.source_inline { content = "x" }
    dest = "relative/dest.txt"
    perm = "0644"
    owner = "root"
    group = "root"
  }
}
`,
		},
		{
			name: "copy_perm_invalid",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.copy {
    src = posix.source_inline { content = "x" }
    dest = "/dest.txt"
    perm = "yolo"
    owner = "root"
    group = "root"
  }
}
`,
		},
		{
			name: "copy_owner_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.copy {
    src = posix.source_inline { content = "x" }
    dest = "/dest.txt"
    perm = "0644"
    owner = ""
    group = "root"
  }
}
`,
		},

		// template
		{
			name: "template_dest_relative",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.template {
    src = posix.source_inline { content = "x" }
    dest = "relative/x"
    perm = "0644"
    owner = "root"
    group = "root"
  }
}
`,
		},

		// dir
		{
			name: "dir_path_relative",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.dir { path = "relative/dir" }
}
`,
		},
		{
			name: "dir_perm_invalid",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.dir {
    path = "/etc/foo"
    perm = "yolo"
  }
}
`,
		},

		// symlink
		{
			name: "symlink_link_relative",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.symlink {
    target = "/etc/source"
    link = "relative/link"
  }
}
`,
		},
		{
			name: "symlink_target_relative",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.symlink {
    target = "relative/source"
    link = "/etc/link"
  }
}
`,
		},

		// mount
		{
			name: "mount_dest_relative",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.mount {
    src = "//server/share"
    dest = "relative/mnt"
    fs_type = posix.MountType.cifs
  }
}
`,
		},
		{
			name: "mount_dest_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.mount {
    src = "//server/share"
    dest = ""
    fs_type = posix.MountType.cifs
  }
}
`,
		},
		{
			name: "mount_src_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.mount {
    src = ""
    dest = "/mnt/data"
    fs_type = posix.MountType.cifs
  }
}
`,
		},

		// pkg
		{
			name: "pkg_packages_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.pkg {
    packages = []
    source = posix.pkg_system {}
  }
}
`,
		},

		// service / user / group / sysctl: name validation
		{
			name: "service_name_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.service { name = "" }
}
`,
		},
		{
			name: "user_name_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.user { name = "" }
}
`,
		},
		{
			name: "group_name_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.group { name = "" }
}
`,
		},
		{
			name: "sysctl_key_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.sysctl { key = "", value = "1" }
}
`,
		},

		// firewall
		{
			name: "firewall_port_invalid_format",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.firewall { port = 99999 }
}
`,
		},

		// run
		{
			name: "run_apply_empty",
			cfg: `
module main
import "std"
import "std/posix"
import "std/local"
let host = local.target { name = "h" }
std.deploy(name = "t", targets = [host]) {
  posix.run { apply = "", check = "true" }
}
`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := source.NewMemSource()
			tgt := target.NewMemTarget()

			rec := &harness.RecordingDisplayer{}
			em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
			store := diagnostic.NewSourceStore()

			if _, err := loadAndResolve(t, c.cfg, src, tgt, em, store); err == nil {
				t.Fatal("expected link-time error, got nil")
			}
		})
	}
}
