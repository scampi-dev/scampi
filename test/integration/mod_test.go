// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/mod"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/test/harness"
)

// Module integration tests
// -----------------------------------------------------------------------------

// setupModCache creates a real temp directory tree that DefaultCacheDir() will
// return when XDG_CACHE_HOME is pointed at the parent.  It returns the module
// directory so callers can populate .scampi files into it.
func setupModCache(t *testing.T, modPath, version string) (cacheDir, modDir string) {
	t.Helper()
	cacheParent := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheParent)

	cacheDir = filepath.Join(cacheParent, "scampi", "mod")
	modDir = filepath.Join(cacheDir, modPath+"@"+version)
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return cacheDir, modDir
}

// writeFile creates a file in dir with the given name and content.
// Returns the full path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
	return path
}

// errContains returns true if the error string contains all of the given substrings.
func errContains(err error, substrings ...string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, s := range substrings {
		if !strings.Contains(msg, s) {
			return false
		}
	}
	return true
}

// Regression test for #310. The cache for an already-fetched module is
// tampered post-fetch; scampi.sum still carries the original hash. apply
// (LoadConfig) must refuse to execute and surface a SumMismatchError
// pointing at the require entry — mirroring what `scampi mod verify`
// would catch manually, but automatically and on every apply.
func TestApply_SumMismatch_AbortsBeforeRun(t *testing.T) {
	const (
		modPath    = "codeberg.org/scampi-modules/helpers"
		modVersion = "v1.0.0"
	)

	_, modDir := setupModCache(t, modPath, modVersion)
	writeFile(t, modDir, "_index.scampi", `module helpers

pub func greeting() string {
  return "hello from module"
}
`)

	// Snapshot the legitimate hash before tampering.
	originalHash, err := mod.ComputeHash(modDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	projDir := t.TempDir()
	writeFile(t, projDir, "scampi.mod", `module codeberg.org/test/myproject

require (
    `+modPath+` `+modVersion+`
)
`)
	writeFile(t, projDir, "scampi.sum", modPath+" "+modVersion+" "+originalHash+"\n")

	// Now tamper the cached module after scampi.sum was committed.
	if err := os.WriteFile(filepath.Join(modDir, "_index.scampi"), []byte(`module helpers

pub func greeting() string {
  return "POISONED"
}
`), 0o644); err != nil {
		t.Fatalf("tamper write: %v", err)
	}

	cfgFile := writeFile(t, projDir, "config.scampi", `module main

import "std"
import "std/local"
import "`+modPath+`"

let host = local.target { name = "localhost" }

std.deploy(name = "test", targets = [host]) {
}
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	_, err = engine.LoadConfig(ctx, em, cfgFile, store, source.LocalPosixSource{})
	if err == nil {
		t.Fatal("expected error from tampered cache, got nil")
	}

	// LoadConfig wraps in AbortError; the SumMismatchError lives in
	// Causes. AbortError doesn't implement Unwrap() []error yet — out
	// of scope for #310.
	var sme *mod.SumMismatchError
	var abort engine.AbortError
	if errors.As(err, &abort) {
		for _, c := range abort.Causes {
			if errors.As(c, &sme) {
				break
			}
		}
	}
	if sme == nil {
		t.Fatalf("expected *mod.SumMismatchError in Causes, got %T: %v", err, err)
	}
	if sme.ModPath != modPath {
		t.Errorf("SumMismatchError.ModPath = %q, want %q", sme.ModPath, modPath)
	}
	if sme.Version != modVersion {
		t.Errorf("SumMismatchError.Version = %q, want %q", sme.Version, modVersion)
	}
}

// TestModuleLoad_Basic verifies that a config can import a function from a
// cached module and use it, producing a valid deploy block.
func TestModuleLoad_Basic(t *testing.T) {
	_, modDir := setupModCache(t, "codeberg.org/scampi-modules/helpers", "v1.0.0")

	if err := os.WriteFile(filepath.Join(modDir, "_index.scampi"), []byte(`
module helpers

pub func greeting() string {
  return "hello from module"
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	projDir := t.TempDir()
	writeFile(t, projDir, "scampi.mod", `module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/helpers v1.0.0
)
`)
	cfgFile := writeFile(t, projDir, "config.scampi", `module main

import "std"
import "std/local"
import "std/posix"
import "codeberg.org/scampi-modules/helpers"

let host = local.target { name = "localhost" }
let msg = helpers.greeting()

std.deploy(name = "test", targets = [host]) {
  posix.dir { path = "/tmp/scampi-mod-test" }
}
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, cfgFile, store, source.LocalPosixSource{})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Deploy) == 0 {
		t.Fatal("expected at least one deploy block")
	}
}

// Module init integration tests
// -----------------------------------------------------------------------------

// TestModInit_CreatesScampiModInEmptyDir verifies the happy path: in an
// empty directory, mod.Init writes scampi.mod with the given module path.
func TestModInit_CreatesScampiModInEmptyDir(t *testing.T) {
	dir := t.TempDir()
	src := source.LocalPosixSource{}

	if err := mod.Init(context.Background(), src, dir, "codeberg.org/test/myproj"); err != nil {
		t.Fatalf("init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "scampi.mod"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "module codeberg.org/test/myproj\n" {
		t.Errorf("scampi.mod content = %q", string(data))
	}
}

// TestModInit_RejectsExistingScampiMod verifies #239: a second init on
// the same directory must fail with "already exists" instead of being a
// no-op or overwriting.
func TestModInit_RejectsExistingScampiMod(t *testing.T) {
	dir := t.TempDir()
	src := source.LocalPosixSource{}
	if err := mod.Init(context.Background(), src, dir, "codeberg.org/test/myproj"); err != nil {
		t.Fatalf("first init: %v", err)
	}

	err := mod.Init(context.Background(), src, dir, "codeberg.org/test/other")
	if err == nil {
		t.Fatal("expected error on second init, got nil")
	}
	if !errContains(err, "already exists") {
		t.Errorf("error = %v, want substring 'already exists'", err)
	}

	// First call's content must still be there.
	data, _ := os.ReadFile(filepath.Join(dir, "scampi.mod"))
	if string(data) != "module codeberg.org/test/myproj\n" {
		t.Errorf("existing scampi.mod was overwritten: got %q", string(data))
	}
}

// TestModuleLoad_Subpath verifies that a subpath import
// (e.g. codeberg.org/user/mod/sub/path) resolves correctly within the cache.
// TODO(#xxx): loadLocalSubmodules only runs for the self-module, not for
// cached external deps. Enable once external subpath scanning is wired up.
func TestModuleLoad_Subpath(t *testing.T) {
	t.Skip("external module subpath imports not yet supported")
	_, modDir := setupModCache(t, "codeberg.org/scampi-modules/toolkit", "v2.3.1")

	// Root module declaration.
	if err := os.WriteFile(filepath.Join(modDir, "_index.scampi"), []byte(`
module toolkit
`), 0o644); err != nil {
		t.Fatalf("WriteFile root: %v", err)
	}

	// Subpath module in net/.
	subDir := filepath.Join(modDir, "net")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "_index.scampi"), []byte(`
module net

pub func make_url(host: string) string {
  return "https://" + host
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	projDir := t.TempDir()
	writeFile(t, projDir, "scampi.mod", `module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/toolkit v2.3.1
)
`)
	cfgFile := writeFile(t, projDir, "config.scampi", `module main

import "std"
import "std/local"
import "std/posix"
import "codeberg.org/scampi-modules/toolkit/net"

let tgt = local.target { name = "host" }
let url = net.make_url(host = "example.com")

std.deploy(name = "net-test", targets = [tgt]) {
  posix.dir { path = "/tmp/net-test" }
}
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, cfgFile, store, source.LocalPosixSource{})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := cfg.DeployByName("net-test"); !ok {
		t.Fatal("expected deploy block 'net-test'")
	}
}

// TestModuleLoad_InternalMultiFile verifies that a module with multiple .scampi
// files sharing the same module declaration can use each other's symbols, and
// that the config can import the module and use its exported functions.
func TestModuleLoad_InternalMultiFile(t *testing.T) {
	_, modDir := setupModCache(t, "codeberg.org/scampi-modules/utils", "v0.1.0")

	// helpers.scampi defines a helper used by _index.scampi.
	if err := os.WriteFile(filepath.Join(modDir, "helpers.scampi"), []byte(`
module utils

pub func add(a: int, b: int) int {
  return a + b
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile helpers.scampi: %v", err)
	}
	// _index.scampi uses add() from the same module and exports double().
	if err := os.WriteFile(filepath.Join(modDir, "_index.scampi"), []byte(`
module utils

pub func double(x: int) int {
  return add(x, x)
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile _index.scampi: %v", err)
	}

	projDir := t.TempDir()
	writeFile(t, projDir, "scampi.mod", `module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/utils v0.1.0
)
`)
	cfgFile := writeFile(t, projDir, "config.scampi", `module main

import "std"
import "std/local"
import "std/posix"
import "codeberg.org/scampi-modules/utils"

let tgt = local.target { name = "host" }
let result = utils.double(x = 21)

std.deploy(name = "utils-test", targets = [tgt]) {
  posix.dir { path = "/tmp/utils-test" }
}
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, cfgFile, store, source.LocalPosixSource{})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := cfg.DeployByName("utils-test"); !ok {
		t.Fatal("expected deploy block 'utils-test'")
	}
}

// TestModuleLoad_NotInRequireTable verifies that importing a module not listed
// in scampi.mod produces an "unknown module" error.
func TestModuleLoad_NotInRequireTable(t *testing.T) {
	cacheParent := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheParent)

	projDir := t.TempDir()
	writeFile(t, projDir, "scampi.mod", `module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/known v1.0.0
)
`)
	cfgFile := writeFile(t, projDir, "config.scampi", `module main

import "std"
import "std/local"
import "std/posix"
import "codeberg.org/scampi-modules/unknown"

let tgt = local.target { name = "host" }

std.deploy(name = "test", targets = [tgt]) {
  posix.dir { path = "/tmp/test" }
}
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	_, err := engine.LoadConfig(ctx, em, cfgFile, store, source.LocalPosixSource{})
	if err == nil {
		t.Fatal("expected LoadConfig to fail for unknown module")
	}
	ae, ok := err.(engine.AbortError)
	if !ok || len(ae.Causes) == 0 {
		t.Fatalf("expected AbortError with causes, got: %v", err)
	}
	if !errContains(ae.Causes[0], "unknown module") {
		t.Errorf("expected 'unknown module' error, got: %v", ae.Causes[0])
	}
}

// TestModuleLoad_NotCached verifies that importing a module that's in the
// require table but not in the cache produces an "unknown module" error.
func TestModuleLoad_NotCached(t *testing.T) {
	cacheParent := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheParent)
	// Deliberately do NOT create the module directory in the cache.

	projDir := t.TempDir()
	writeFile(t, projDir, "scampi.mod", `module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/missing v3.0.0
)
`)
	cfgFile := writeFile(t, projDir, "config.scampi", `module main

import "std"
import "std/local"
import "std/posix"
import "codeberg.org/scampi-modules/missing"

let tgt = local.target { name = "host" }

std.deploy(name = "test", targets = [tgt]) {
  posix.dir { path = "/tmp/test" }
}
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	_, err := engine.LoadConfig(ctx, em, cfgFile, store, source.LocalPosixSource{})
	if err == nil {
		t.Fatal("expected LoadConfig to fail for uncached module")
	}
	ae, ok := err.(engine.AbortError)
	if !ok || len(ae.Causes) == 0 {
		t.Fatalf("expected AbortError with causes, got: %v", err)
	}
	if !errContains(ae.Causes[0], "unknown module") {
		t.Errorf("expected 'unknown module' error, got: %v", ae.Causes[0])
	}
}

// createEmptyRepo creates a bare git repo tagged at v1.0.0 with no .scampi files.
func createEmptyRepo(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	bare := filepath.Join(t.TempDir(), "empty.git")

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	runGit(work, "init")
	runGit(work, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("not a module\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(work, "add", ".")
	runGit(work, "commit", "-m", "initial")
	runGit(work, "tag", "v1.0.0")
	runGit(work, "clone", "--bare", work, bare)
	return bare
}

// TestModuleAdd_ThenLoad exercises the full add+load pipeline: create a git
// repo as a module, pre-populate the cache (skipping the real network fetch),
// use mod.Add to register it in scampi.mod and scampi.sum, then verify
// engine.LoadConfig can load a config that imports from it.
//
// The cache is pre-populated so the test doesn't require internet access;
// the actual git-clone path is covered by TestFetch_* in mod/fetch_test.go.
func TestModuleAdd_ThenLoad(t *testing.T) {
	const modPath = "codeberg.org/test/greetings"
	const version = "v1.0.0"

	projDir := t.TempDir()
	cacheParent := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheParent)

	// Pre-seed the cache so mod.Add skips the git clone and goes straight to
	// validation + hash + writing scampi.mod/scampi.sum.
	cacheDir := mod.DefaultCacheDir()
	cachedModDir := filepath.Join(cacheDir, modPath+"@"+version)
	if err := os.MkdirAll(cachedModDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachedModDir, "_index.scampi"), []byte(`
module greetings

pub func greet(name: string) string {
  return "hello, " + name
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	modContent := "module codeberg.org/test/addload\n"
	if err := os.WriteFile(filepath.Join(projDir, "scampi.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	resolvedVersion, _, err := mod.Add(
		context.Background(),
		source.LocalPosixSource{},
		modPath,
		version,
		projDir,
		cacheDir,
	)
	if err != nil {
		t.Fatalf("mod.Add: %v", err)
	}
	if resolvedVersion != version {
		t.Errorf("mod.Add returned version %q, want %q", resolvedVersion, version)
	}

	if _, err := os.Stat(filepath.Join(projDir, "scampi.sum")); err != nil {
		t.Fatalf("scampi.sum not created after mod.Add: %v", err)
	}

	cfgFile := writeFile(t, projDir, "config.scampi", `module main

import "std"
import "std/local"
import "std/posix"
import "codeberg.org/test/greetings"

let host = local.target { name = "localhost" }
let msg = greetings.greet(name = "world")

std.deploy(name = "add-load-test", targets = [host]) {
  posix.dir { path = "/tmp/add-load-test" }
}
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, cfgFile, store, source.LocalPosixSource{})
	if err != nil {
		t.Fatalf("engine.LoadConfig: %v", err)
	}
	if _, ok := cfg.DeployByName("add-load-test"); !ok {
		t.Fatal("expected deploy block 'add-load-test'")
	}
}

// TestModuleAdd_NotAModule verifies that adding a repo with no .scampi files
// fails with NotAModuleError. This is the one mod_test scenario that actually
// shells out to `git` (via createEmptyRepo + the mod.Add clone). Other module
// tests pre-seed the cache to skip the real fetch path; the actual git-clone
// path is covered separately by TestFetch_* in mod/fetch_test.go.
func TestModuleAdd_NotAModule(t *testing.T) {
	repoPath := createEmptyRepo(t)

	projDir := t.TempDir()
	cacheParent := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheParent)

	modContent := "module codeberg.org/test/notamod\n"
	if err := os.WriteFile(filepath.Join(projDir, "scampi.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	src := source.LocalPosixSource{}
	_, _, err := mod.Add(ctx, src, repoPath, "v1.0.0", projDir, mod.DefaultCacheDir())
	if err == nil {
		t.Fatal("expected NotAModuleError, got nil")
	}

	var nme *mod.NotAModuleError
	if !errors.As(err, &nme) {
		t.Fatalf("expected *mod.NotAModuleError, got %T: %v", err, err)
	}
	if nme.ModPath != repoPath {
		t.Errorf("NotAModuleError.ModPath = %q, want %q", nme.ModPath, repoPath)
	}
}

// TestModuleLoad_NoModFile verifies backward compatibility: a config with no
// scampi.mod and only built-in steps still works correctly.
func TestModuleLoad_NoModFile(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(`
module main

import "std"
import "std/local"
import "std/posix"

let host = local.target { name = "host" }

std.deploy(name = "compat-test", targets = [host]) {
  posix.dir { path = "/tmp/compat-test" }
}
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("LoadConfig without scampi.mod failed: %v", err)
	}
	if _, ok := cfg.DeployByName("compat-test"); !ok {
		t.Fatal("expected deploy block 'compat-test'")
	}
}

// TestModuleLoad_Local verifies that local modules (version is a relative
// path) resolve directly from the filesystem without going through the cache.
func TestModuleLoad_Local(t *testing.T) {
	projDir := t.TempDir()

	// Create a local module directory
	localMod := filepath.Join(projDir, "modules", "helpers")
	if err := os.MkdirAll(localMod, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localMod, "_index.scampi"), []byte(`
module helpers

pub func greet(name: string) string {
  return "hello, " + name
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	modFile := filepath.Join(projDir, "scampi.mod")
	if err := os.WriteFile(modFile, []byte(`module codeberg.org/test/proj

require (
	my/helpers ./modules/helpers
)
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgFile := filepath.Join(projDir, "config.scampi")
	if err := os.WriteFile(cfgFile, []byte(`module main

import "std"
import "std/local"
import "std/posix"
import "my/helpers"

let tgt = local.target { name = "host" }
let msg = helpers.greet(name = "world")

std.deploy(name = "local-mod-test", targets = [tgt]) {
  posix.dir { path = "/tmp/local-mod-test" }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	src := source.LocalPosixSource{}
	cfg, err := engine.LoadConfig(ctx, em, cfgFile, store, src)
	if err != nil {
		t.Fatalf("LoadConfig with local module: %v", err)
	}
	if _, ok := cfg.DeployByName("local-mod-test"); !ok {
		t.Fatal("expected deploy block 'local-mod-test'")
	}
}

// TestModuleLoad_LocalAbsPath verifies that absolute-path local modules work.
func TestModuleLoad_LocalAbsPath(t *testing.T) {
	projDir := t.TempDir()
	localMod := t.TempDir()

	if err := os.WriteFile(filepath.Join(localMod, "_index.scampi"), []byte(`
module util

pub func helper() int {
  return 42
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	modFile := filepath.Join(projDir, "scampi.mod")
	modContent := "module codeberg.org/test/proj\n\n" +
		"require (\n\tmy/util " + localMod + "\n)\n"
	if err := os.WriteFile(modFile, []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgFile := filepath.Join(projDir, "config.scampi")
	if err := os.WriteFile(cfgFile, []byte(`module main

import "std"
import "std/local"
import "std/posix"
import "my/util"

let tgt = local.target { name = "host" }
let val = util.helper()

std.deploy(name = "abs-test", targets = [tgt]) {
  posix.dir { path = "/tmp/abs-test" }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	src := source.LocalPosixSource{}
	cfg, err := engine.LoadConfig(ctx, em, cfgFile, store, src)
	if err != nil {
		t.Fatalf("LoadConfig with absolute local module: %v", err)
	}
	if _, ok := cfg.DeployByName("abs-test"); !ok {
		t.Fatal("expected deploy block 'abs-test'")
	}
}
