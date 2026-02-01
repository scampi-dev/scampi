package test

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

// E2EScenario defines a data-driven E2E test case.
// Each scenario is a directory under testdata/e2e/ containing:
//   - config.cue     (required) - the doit configuration
//   - source.json    (required) - source files to populate
//   - target.json    (optional) - pre-existing target state
//   - expect.json    (required) - expected outcomes
type E2EScenario struct {
	Source E2EFiles  `json:"source"`
	Target E2EFiles  `json:"target"`
	Expect E2EExpect `json:"expect"`
}

// E2EFiles represents a virtual filesystem as a map of path -> content.
type E2EFiles struct {
	Files    map[string]string   `json:"files"`
	Perms    map[string]string   `json:"perms,omitempty"`    // path -> "0644"
	Owners   map[string]E2EOwner `json:"owners,omitempty"`   // path -> owner info
	Symlinks map[string]string   `json:"symlinks,omitempty"` // link -> target
}

// E2EOwner represents file ownership.
type E2EOwner struct {
	User  string `json:"user"`
	Group string `json:"group"`
}

// E2EExpect defines expected outcomes after running the engine.
type E2EExpect struct {
	// Target is the expected target filesystem state after execution.
	Target E2EFiles `json:"target"`

	// Changed is the expected number of changed operations.
	Changed int `json:"changed"`

	// Error, if true, expects the engine to return an error.
	Error bool `json:"error,omitempty"`

	// Diagnostics lists expected diagnostic template IDs (optional).
	Diagnostics []string `json:"diagnostics,omitempty"`
}

func TestE2E(t *testing.T) {
	root := absPath("testdata/e2e")

	entries, err := readDirSafe(root)
	if err != nil {
		t.Skip("testdata/e2e directory not found")
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		t.Run(name, func(t *testing.T) {
			runE2EScenario(t, filepath.Join(root, name))
		})
	}
}

func runE2EScenario(t *testing.T, dir string) {
	cfgPath := filepath.Join(dir, "config.cue")
	sourcePath := filepath.Join(dir, "source.json")
	targetPath := filepath.Join(dir, "target.json")
	expectPath := filepath.Join(dir, "expect.json")

	// Load source files
	srcFiles := loadE2EFiles(t, sourcePath)

	// Load pre-existing target files (optional)
	tgtFiles := E2EFiles{Files: map[string]string{}}
	if data, err := readFileSafe(targetPath); err == nil {
		if err := json.Unmarshal(data, &tgtFiles); err != nil {
			t.Fatalf("failed to parse %s: %v", targetPath, err)
		}
		if tgtFiles.Files == nil {
			tgtFiles.Files = map[string]string{}
		}
	}

	// Load expected outcomes
	expect := loadE2EExpect(t, expectPath)

	// Build MemSource with config + source files
	src := source.NewMemSource()
	cfgData := readOrDie(cfgPath)
	src.Files["/config.cue"] = cfgData
	for path, content := range srcFiles.Files {
		src.Files[path] = []byte(content)
	}

	// Build MemTarget with pre-existing state
	tgt := target.NewMemTarget()
	for path, content := range tgtFiles.Files {
		tgt.Files[path] = []byte(content)
	}
	for path, permStr := range tgtFiles.Perms {
		perm := parsePermOrDie(t, permStr)
		tgt.Modes[path] = perm
	}
	for path, owner := range tgtFiles.Owners {
		tgt.Owners[path] = target.Owner{User: owner.User, Group: owner.Group}
	}
	for link, linkTarget := range tgtFiles.Symlinks {
		tgt.Symlinks[link] = linkTarget
	}

	// Run engine
	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	apply := func() error {
		ctx := context.Background()
		cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
		if err != nil {
			return err
		}

		cfg.Target = mockTargetInstance(tgt)

		e, err := engine.New(ctx, src, cfg, em)
		if err != nil {
			return err
		}
		defer e.Close()

		return e.Apply(ctx)
	}

	err := apply()

	// Assert error expectation
	if expect.Error {
		if err == nil {
			t.Fatalf("expected error, got success\n%s", rec)
		}
	} else {
		if err != nil {
			t.Fatalf("expected success, got error: %v\n%s", err, rec)
		}
	}

	// Assert target file contents
	for path, wantContent := range expect.Target.Files {
		got, ok := tgt.Files[path]
		if !ok {
			t.Errorf("expected target file %q to exist", path)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("target file %q: got %q, want %q", path, got, wantContent)
		}
	}

	// Assert target permissions (if specified)
	for path, wantPermStr := range expect.Target.Perms {
		wantPerm := parsePermOrDie(t, wantPermStr)
		gotPerm, ok := tgt.Modes[path]
		if !ok {
			t.Errorf("expected target file %q to have permissions", path)
			continue
		}
		if gotPerm != wantPerm {
			t.Errorf("target file %q perms: got %o, want %o", path, gotPerm, wantPerm)
		}
	}

	// Assert target symlinks (if specified)
	for link, wantTarget := range expect.Target.Symlinks {
		gotTarget, ok := tgt.Symlinks[link]
		if !ok {
			t.Errorf("expected symlink %q to exist", link)
			continue
		}
		if gotTarget != wantTarget {
			t.Errorf("symlink %q: got target %q, want %q", link, gotTarget, wantTarget)
		}
	}

	// Assert changed count
	gotChanged := rec.countChangedOps()
	if gotChanged != expect.Changed {
		t.Errorf("changed count: got %d, want %d", gotChanged, expect.Changed)
	}

	// Assert diagnostics (if specified)
	if len(expect.Diagnostics) > 0 {
		gotDiags := rec.collectDiagnosticIDs()
		if !stringSlicesEqual(gotDiags, expect.Diagnostics) {
			t.Errorf("diagnostics: got %v, want %v", gotDiags, expect.Diagnostics)
		}
	}
}

func loadE2EFiles(t *testing.T, path string) E2EFiles {
	t.Helper()
	data := readOrDie(path)
	var f E2EFiles
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("failed to parse %s: %v", path, err)
	}
	if f.Files == nil {
		f.Files = map[string]string{}
	}
	return f
}

func loadE2EExpect(t *testing.T, path string) E2EExpect {
	t.Helper()
	data := readOrDie(path)
	var e E2EExpect
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("failed to parse %s: %v", path, err)
	}
	if e.Target.Files == nil {
		e.Target.Files = map[string]string{}
	}
	return e
}

func parsePermOrDie(t *testing.T, s string) fs.FileMode {
	t.Helper()
	var perm uint32
	if _, err := parseOctal(s, &perm); err != nil {
		t.Fatalf("invalid permission %q: %v", s, err)
	}
	return fs.FileMode(perm)
}

func parseOctal(s string, out *uint32) (bool, error) {
	var v uint32
	for _, c := range s {
		if c < '0' || c > '7' {
			return false, nil
		}
		v = v*8 + uint32(c-'0')
	}
	*out = v
	return true, nil
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
