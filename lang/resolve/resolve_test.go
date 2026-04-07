// SPDX-License-Identifier: GPL-3.0-only

package resolve

import (
	"testing"
	"testing/fstest"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/std"
)

func stdModules(t *testing.T) map[string]*check.Scope {
	t.Helper()
	modules, err := check.BootstrapStd(std.FS)
	if err != nil {
		t.Fatalf("bootstrap std: %v", err)
	}
	return modules
}

func TestResolveIntraProject(t *testing.T) {
	root := fstest.MapFS{
		"targets.scampi": &fstest.MapFile{
			Data: []byte(`
module targets
let host = "10.0.0.1"
`),
		},
	}
	r := New(Config{
		ModulePath: "example.com/myproject",
		RootFS:     root,
		StdModules: stdModules(t),
	})
	m := r.Resolve("example.com/myproject/targets")
	if m == nil {
		t.Fatalf("resolve failed: %v", r.Errors())
	}
	if m.Scope == nil {
		t.Fatal("scope is nil")
	}
	if m.Scope.Lookup("host") == nil {
		t.Error("expected 'host' in exported scope")
	}
}

func TestResolveStdModule(t *testing.T) {
	r := New(Config{
		ModulePath: "example.com/myproject",
		StdModules: stdModules(t),
	})
	m := r.Resolve("std")
	if m == nil {
		t.Fatalf("resolve std failed: %v", r.Errors())
	}
	m2 := r.Resolve("std/posix")
	if m2 == nil {
		t.Fatalf("resolve std/posix failed: %v", r.Errors())
	}
}

func TestResolveLocalDep(t *testing.T) {
	root := fstest.MapFS{
		"modules/utils.scampi": &fstest.MapFile{
			Data: []byte(`
module modules
func helper() string { return "ok" }
`),
		},
	}
	r := New(Config{
		ModulePath: "example.com/myproject",
		RootFS:     root,
		Deps: []Dependency{
			{
				Path:      "example.com/shared/modules",
				Version:   "v1.0.0",
				LocalPath: "modules",
			},
		},
		StdModules: stdModules(t),
	})
	m := r.Resolve("example.com/shared/modules/utils")
	if m == nil {
		t.Fatalf("resolve failed: %v", r.Errors())
	}
	if m.Scope.Lookup("helper") == nil {
		t.Error("expected 'helper' in exported scope")
	}
}

func TestResolveRemoteDep(t *testing.T) {
	cache := fstest.MapFS{
		"example.com/lib@v2.0.0/core.scampi": &fstest.MapFile{
			Data: []byte(`
module lib
type Config { name: string }
`),
		},
	}
	r := New(Config{
		ModulePath: "example.com/myproject",
		CacheFS:    cache,
		Deps: []Dependency{
			{Path: "example.com/lib", Version: "v2.0.0"},
		},
		StdModules: stdModules(t),
	})
	m := r.Resolve("example.com/lib/core")
	if m == nil {
		t.Fatalf("resolve failed: %v", r.Errors())
	}
	if m.Scope.Lookup("Config") == nil {
		t.Error("expected 'Config' in exported scope")
	}
}

func TestResolveNotFound(t *testing.T) {
	r := New(Config{
		ModulePath: "example.com/myproject",
		RootFS:     fstest.MapFS{},
		StdModules: map[string]*check.Scope{},
	})
	m := r.Resolve("example.com/nonexistent")
	if m != nil {
		t.Error("expected nil for nonexistent module")
	}
	if len(r.Errors()) == 0 {
		t.Error("expected error")
	}
}

func TestResolveCached(t *testing.T) {
	root := fstest.MapFS{
		"utils.scampi": &fstest.MapFile{
			Data: []byte(`
module utils
let x = 1
`),
		},
	}
	r := New(Config{
		ModulePath: "example.com/myproject",
		RootFS:     root,
		StdModules: stdModules(t),
	})
	m1 := r.Resolve("example.com/myproject/utils")
	m2 := r.Resolve("example.com/myproject/utils")
	if m1 != m2 {
		t.Error("second resolve should return cached result")
	}
}

func TestResolveDirectory(t *testing.T) {
	root := fstest.MapFS{
		"targets/ssh.scampi": &fstest.MapFile{
			Data: []byte(`
module targets
let vps_host = "10.0.0.1"
`),
		},
		"targets/rest.scampi": &fstest.MapFile{
			Data: []byte(`
module targets
let api_url = "https://api.example.com"
`),
		},
	}
	r := New(Config{
		ModulePath: "example.com/myproject",
		RootFS:     root,
		StdModules: stdModules(t),
	})
	m := r.Resolve("example.com/myproject/targets")
	if m == nil {
		t.Fatalf("resolve dir failed: %v", r.Errors())
	}
	if m.Scope.Lookup("vps_host") == nil {
		t.Error("expected 'vps_host' from ssh.scampi")
	}
	if m.Scope.Lookup("api_url") == nil {
		t.Error("expected 'api_url' from rest.scampi")
	}
}

func TestResolveDirPrecedence(t *testing.T) {
	root := fstest.MapFS{
		"targets/main.scampi": &fstest.MapFile{
			Data: []byte(`
module targets
let from_dir = true
`),
		},
		"targets.scampi": &fstest.MapFile{
			Data: []byte(`
module targets
let from_file = true
`),
		},
	}
	r := New(Config{
		ModulePath: "example.com/myproject",
		RootFS:     root,
		StdModules: stdModules(t),
	})
	m := r.Resolve("example.com/myproject/targets")
	if m == nil {
		t.Fatalf("resolve failed: %v", r.Errors())
	}
	if m.Scope.Lookup("from_dir") == nil {
		t.Error("directory should take precedence")
	}
	if m.Scope.Lookup("from_file") != nil {
		t.Error("file should be ignored when directory exists")
	}
}
