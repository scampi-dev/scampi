// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"os"
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
