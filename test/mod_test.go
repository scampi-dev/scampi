// SPDX-License-Identifier: GPL-3.0-only

package test

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
)

// Module integration tests
// -----------------------------------------------------------------------------

// diskFallbackSource wraps a MemSource and falls back to the real filesystem
// for absolute paths that are not present in the virtual files map.  This is
// needed because module .star files live on disk (inside t.TempDir) while the
// user config and scampi.mod live only in memory.
type diskFallbackSource struct {
	mem *source.MemSource
}

func (d *diskFallbackSource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	data, err := d.mem.ReadFile(ctx, path)
	if err == nil {
		return data, nil
	}
	if filepath.IsAbs(path) {
		return os.ReadFile(path)
	}
	return nil, err
}

func (d *diskFallbackSource) WriteFile(ctx context.Context, path string, data []byte) error {
	return d.mem.WriteFile(ctx, path, data)
}

func (d *diskFallbackSource) EnsureDir(ctx context.Context, path string) error {
	return d.mem.EnsureDir(ctx, path)
}

func (d *diskFallbackSource) Stat(ctx context.Context, path string) (source.FileMeta, error) {
	meta, err := d.mem.Stat(ctx, path)
	if err != nil {
		return source.FileMeta{}, err
	}
	if meta.Exists {
		return meta, nil
	}
	if filepath.IsAbs(path) {
		info, statErr := os.Stat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				return source.FileMeta{Exists: false}, nil
			}
			return source.FileMeta{}, statErr
		}
		return source.FileMeta{
			Exists:   true,
			IsDir:    info.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime(),
		}, nil
	}
	return source.FileMeta{Exists: false}, nil
}

func (d *diskFallbackSource) LookupEnv(key string) (string, bool) {
	return d.mem.LookupEnv(key)
}

func (d *diskFallbackSource) LookupSecret(key string) (string, bool, error) {
	return d.mem.LookupSecret(key)
}

// Ensure diskFallbackSource implements source.Source at compile time.
var _ source.Source = (*diskFallbackSource)(nil)

// modMemSrc returns a diskFallbackSource with a MemSource pre-loaded with a
// scampi.mod and a config.star.  Module .star files on the real filesystem
// are accessible through the disk fallback.
func modMemSrc(modFile, configFile string) *diskFallbackSource {
	mem := source.NewMemSource()
	mem.Files["/scampi.mod"] = []byte(modFile)
	mem.Files["/config.star"] = []byte(configFile)
	return &diskFallbackSource{mem: mem}
}

// setupModCache creates a real temp directory tree that DefaultCacheDir() will
// return when XDG_CACHE_HOME is pointed at the parent.  It returns the module
// directory so callers can populate .star files into it.
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

func loadCfgSrc(t *testing.T, src source.Source) (engine.AbortError, bool) {
	t.Helper()
	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		return engine.AbortError{}, false
	}
	if ae, ok := err.(engine.AbortError); ok {
		return ae, true
	}
	return engine.AbortError{Causes: []error{err}}, true
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

// TestModuleLoad_Basic verifies that a config can load a function from a
// module and use it, producing a valid deploy block.
func TestModuleLoad_Basic(t *testing.T) {
	_, modDir := setupModCache(t, "codeberg.org/scampi-modules/helpers", "v1.0.0")

	if err := os.WriteFile(filepath.Join(modDir, "_index.star"), []byte(`
def greeting():
    return "hello from module"
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	src := modMemSrc(
		`module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/helpers v1.0.0
)
`,
		`load("codeberg.org/scampi-modules/helpers", "greeting")

msg = greeting()

target.local(name="localhost")

deploy(
    name="test",
    targets=["localhost"],
    steps=[
        dir(path="/tmp/scampi-mod-test"),
    ],
)
`,
	)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Deploy) == 0 {
		t.Fatal("expected at least one deploy block")
	}
}

// TestModuleLoad_Subpath verifies that a subpath load
// (e.g. codeberg.org/user/mod/sub/path) resolves correctly within the cache.
func TestModuleLoad_Subpath(t *testing.T) {
	_, modDir := setupModCache(t, "codeberg.org/scampi-modules/toolkit", "v2.3.1")

	subDir := filepath.Join(modDir, "net")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "helpers.star"), []byte(`
def make_url(host):
    return "https://" + host
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	src := modMemSrc(
		`module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/toolkit v2.3.1
)
`,
		`load("codeberg.org/scampi-modules/toolkit/net/helpers", "make_url")

url = make_url("example.com")

target.local(name="host")

deploy(
    name="net-test",
    targets=["host"],
    steps=[
        dir(path="/tmp/net-test"),
    ],
)
`,
	)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := cfg.Deploy["net-test"]; !ok {
		t.Fatal("expected deploy block 'net-test'")
	}
}

// TestModuleLoad_InternalRelativeLoad verifies that a module's _index.star can
// load a sibling file relatively, and that the relative load resolves within
// the module's cache directory rather than the user's config directory.
func TestModuleLoad_InternalRelativeLoad(t *testing.T) {
	_, modDir := setupModCache(t, "codeberg.org/scampi-modules/utils", "v0.1.0")

	if err := os.WriteFile(filepath.Join(modDir, "helpers.star"), []byte(`
def add(a, b):
    return a + b
`), 0o644); err != nil {
		t.Fatalf("WriteFile helpers.star: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "_index.star"), []byte(`
load("helpers.star", "add")

def double(x):
    return add(x, x)
`), 0o644); err != nil {
		t.Fatalf("WriteFile _index.star: %v", err)
	}

	src := modMemSrc(
		`module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/utils v0.1.0
)
`,
		`load("codeberg.org/scampi-modules/utils", "double")

result = double(21)

target.local(name="host")

deploy(
    name="utils-test",
    targets=["host"],
    steps=[
        dir(path="/tmp/utils-test"),
    ],
)
`,
	)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := cfg.Deploy["utils-test"]; !ok {
		t.Fatal("expected deploy block 'utils-test'")
	}
}

// TestModuleLoad_NotInRequireTable verifies that loading a module not listed in
// scampi.mod produces an error mentioning "not found".
func TestModuleLoad_NotInRequireTable(t *testing.T) {
	cacheParent := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheParent)

	src := modMemSrc(
		`module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/known v1.0.0
)
`,
		`load("codeberg.org/scampi-modules/unknown", "something")

target.local(name="host")

deploy(
    name="test",
    targets=["host"],
    steps=[
        dir(path="/tmp/test"),
    ],
)
`,
	)

	ae, failed := loadCfgSrc(t, src)
	if !failed {
		t.Fatal("expected LoadConfig to fail for unknown module")
	}
	if len(ae.Causes) == 0 {
		t.Fatal("expected at least one cause")
	}

	found := false
	for _, cause := range ae.Causes {
		if _, ok := cause.(*mod.ModuleNotFoundError); ok {
			found = true
			break
		}
		if errContains(cause, "not found") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a ModuleNotFoundError in causes, got: %v", ae.Causes)
	}
}

// TestModuleLoad_NotCached verifies that loading a module that's in the
// require table but not in the cache produces an error about it not being cached.
func TestModuleLoad_NotCached(t *testing.T) {
	cacheParent := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheParent)
	// Deliberately do NOT create the module directory in the cache.

	src := modMemSrc(
		`module codeberg.org/test/myproject

require (
    codeberg.org/scampi-modules/missing v3.0.0
)
`,
		`load("codeberg.org/scampi-modules/missing", "something")

target.local(name="host")

deploy(
    name="test",
    targets=["host"],
    steps=[
        dir(path="/tmp/test"),
    ],
)
`,
	)

	ae, failed := loadCfgSrc(t, src)
	if !failed {
		t.Fatal("expected LoadConfig to fail for uncached module")
	}
	if len(ae.Causes) == 0 {
		t.Fatal("expected at least one cause")
	}

	found := false
	for _, cause := range ae.Causes {
		if _, ok := cause.(*mod.ModuleNotCachedError); ok {
			found = true
			break
		}
		if errContains(cause, "not cached") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a ModuleNotCachedError in causes, got: %v", ae.Causes)
	}
}

// createModuleRepo creates a bare git repo tagged at v1.0.0 containing an
// _index.star that exports a simple function.  Returns the bare repo path.
func createModuleRepo(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	bare := filepath.Join(t.TempDir(), "mod.git")

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
	if err := os.WriteFile(filepath.Join(work, "_index.star"), []byte(`
def greet(name):
    return "hello, " + name
`), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(work, "add", ".")
	runGit(work, "commit", "-m", "initial")
	runGit(work, "tag", "v1.0.0")
	runGit(work, "clone", "--bare", work, bare)
	return bare
}

// createEmptyRepo creates a bare git repo tagged at v1.0.0 with no .star files.
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

	repoPath := createModuleRepo(t)
	_ = repoPath // bare repo created; cache pre-seeded below

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
	if err := os.WriteFile(filepath.Join(cachedModDir, "_index.star"), []byte(`
def greet(name):
    return "hello, " + name
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

	updatedMod, err := os.ReadFile(filepath.Join(projDir, "scampi.mod"))
	if err != nil {
		t.Fatal(err)
	}

	configStar := `load("codeberg.org/test/greetings", "greet")

msg = greet("world")

target.local(name="localhost")

deploy(
    name="add-load-test",
    targets=["localhost"],
    steps=[
        dir(path="/tmp/add-load-test"),
    ],
)
`

	src := modMemSrc(string(updatedMod), configStar)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig: %v", err)
	}
	if _, ok := cfg.Deploy["add-load-test"]; !ok {
		t.Fatal("expected deploy block 'add-load-test'")
	}
}

// TestModuleAdd_NotAModule verifies that adding a repo with no .star files
// fails with NotAModuleError.
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
	src.Files["/config.star"] = []byte(`
target.local(name="host")

deploy(
    name="compat-test",
    targets=["host"],
    steps=[
        dir(path="/tmp/compat-test"),
    ],
)
`)

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("LoadConfig without scampi.mod failed: %v", err)
	}
	if _, ok := cfg.Deploy["compat-test"]; !ok {
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
	if err := os.WriteFile(filepath.Join(localMod, "_index.star"), []byte(`
def greet(name):
    return "hello, " + name
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

	cfgFile := filepath.Join(projDir, "config.star")
	if err := os.WriteFile(cfgFile, []byte(`load("my/helpers", "greet")

msg = greet("world")

target.local(name="host")

deploy(
    name="local-mod-test",
    targets=["host"],
    steps=[
        dir(path="/tmp/local-mod-test"),
    ],
)
`), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	src := source.LocalPosixSource{}
	cfg, err := engine.LoadConfig(ctx, em, cfgFile, store, src)
	if err != nil {
		t.Fatalf("LoadConfig with local module: %v", err)
	}
	if _, ok := cfg.Deploy["local-mod-test"]; !ok {
		t.Fatal("expected deploy block 'local-mod-test'")
	}
}

// TestModuleLoad_LocalAbsPath verifies that absolute-path local modules work.
func TestModuleLoad_LocalAbsPath(t *testing.T) {
	projDir := t.TempDir()
	localMod := t.TempDir()

	if err := os.WriteFile(filepath.Join(localMod, "_index.star"), []byte(`
def helper():
    return 42
`), 0o644); err != nil {
		t.Fatal(err)
	}

	modFile := filepath.Join(projDir, "scampi.mod")
	modContent := "module codeberg.org/test/proj\n\n" +
		"require (\n\tmy/util " + localMod + "\n)\n"
	if err := os.WriteFile(modFile, []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgFile := filepath.Join(projDir, "config.star")
	if err := os.WriteFile(cfgFile, []byte(`load("my/util", "helper")

val = helper()

target.local(name="host")

deploy(
    name="abs-test",
    targets=["host"],
    steps=[
        dir(path="/tmp/abs-test"),
    ],
)
`), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store := diagnostic.NewSourceStore()
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)

	src := source.LocalPosixSource{}
	cfg, err := engine.LoadConfig(ctx, em, cfgFile, store, src)
	if err != nil {
		t.Fatalf("LoadConfig with absolute local module: %v", err)
	}
	if _, ok := cfg.Deploy["abs-test"]; !ok {
		t.Fatal("expected deploy block 'abs-test'")
	}
}
