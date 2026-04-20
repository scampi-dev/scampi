// SPDX-License-Identifier: GPL-3.0-only

package e2e

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/test/harness"
)

// E2EScenario defines a data-driven E2E test case.
// Each scenario is a directory under testdata/e2e/ containing:
//   - config.scampi    (required) - scampi configuration
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
	Files           map[string]string           `json:"files"`
	Dirs            map[string]bool             `json:"dirs,omitempty"`            // path -> exists
	Perms           map[string]string           `json:"perms,omitempty"`           // path -> "0644"
	Owners          map[string]E2EOwner         `json:"owners,omitempty"`          // path -> owner info
	Symlinks        map[string]string           `json:"symlinks,omitempty"`        // link -> target
	Pkgs            map[string]bool             `json:"pkgs,omitempty"`            // pkg name -> installed
	Upgradable      map[string]bool             `json:"upgradable,omitempty"`      // pkg name -> has upgrade
	Services        map[string]bool             `json:"services,omitempty"`        // service name -> active (running)
	EnabledServices map[string]bool             `json:"enabledServices,omitempty"` // service name -> enabled at boot
	Restarts        map[string]int              `json:"restarts,omitempty"`        // service name -> restart call count
	Reloads         map[string]int              `json:"reloads,omitempty"`         // service name -> reload call count
	Commands        map[string]E2ECommandResult `json:"commands,omitempty"`        // cmd string -> result
	CacheStale      bool                        `json:"cacheStale,omitempty"`      // simulate stale package cache
	Users           map[string]E2EUserInfo      `json:"users,omitempty"`           // username -> user info
	Groups          map[string]E2EGroupInfo     `json:"groups,omitempty"`          // group name -> group info
	Repos           map[string]bool             `json:"repos,omitempty"`           // repo name -> configured
	RepoKeys        map[string]bool             `json:"repoKeys,omitempty"`        // repo name -> key installed
	VersionCodename string                      `json:"versionCodename,omitempty"` // e.g. "bookworm"
}

// E2EOwner represents file ownership.
type E2EOwner struct {
	User  string `json:"user"`
	Group string `json:"group"`
}

// E2EUserInfo represents a user account.
type E2EUserInfo struct {
	Shell    string   `json:"shell,omitempty"`
	Home     string   `json:"home,omitempty"`
	System   bool     `json:"system,omitempty"`
	Password string   `json:"password,omitempty"`
	Groups   []string `json:"groups,omitempty"`
}

// E2EGroupInfo represents a group.
type E2EGroupInfo struct {
	GID    int  `json:"gid,omitempty"`
	System bool `json:"system,omitempty"`
}

// E2ECommandResult defines the simulated result of a shell command.
// If After is set, it becomes the result after any other command has run
// (simulating state change from an apply command).
type E2ECommandResult struct {
	ExitCode int               `json:"exitCode"`
	Stdout   string            `json:"stdout,omitempty"`
	Stderr   string            `json:"stderr,omitempty"`
	After    *E2ECommandResult `json:"after,omitempty"`
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

	// MemOnly skips non-mem drivers (for scenarios that require simulated state).
	MemOnly bool `json:"memOnly,omitempty"`
}

func TestE2E(t *testing.T) {
	root := harness.AbsPath("../testdata/e2e")

	entries, err := harness.ReadDirSafe(root)
	if err != nil {
		t.Skip("../testdata/e2e directory not found")
	}

	// Get all available drivers
	drivers := AllDrivers(t)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		scenarioDir := filepath.Join(root, name)

		cfgPath := filepath.Join(scenarioDir, "config.scampi")
		if _, err := harness.ReadFileSafe(cfgPath); err != nil {
			t.Errorf("%s: no config.scampi found", name)
			continue
		}

		for _, driver := range drivers {
			t.Run(name+"/"+driver.Name(), func(t *testing.T) {
				runE2EScenarioWithDriver(t, scenarioDir, "config.scampi", driver)
			})
		}
	}
}

func runE2EScenarioWithDriver(t *testing.T, dir string, cfgFilename string, driver E2EDriver) {
	cfgPath := filepath.Join(dir, cfgFilename)
	sourcePath := filepath.Join(dir, "source.json")
	targetPath := filepath.Join(dir, "target.json")
	expectPath := filepath.Join(dir, "expect.json")

	// Load source files
	srcFiles := loadE2EFiles(t, sourcePath)

	// Load pre-existing target files (optional)
	tgtFiles := E2EFiles{Files: map[string]string{}}
	if data, err := harness.ReadFileSafe(targetPath); err == nil {
		if err := json.Unmarshal(data, &tgtFiles); err != nil {
			t.Fatalf("failed to parse %s: %v", targetPath, err)
		}
		if tgtFiles.Files == nil {
			tgtFiles.Files = map[string]string{}
		}
	}

	// Load expected outcomes
	expect := loadE2EExpect(t, expectPath)

	if expect.MemOnly && driver.Name() != "mem" {
		t.Skip("scenario requires simulated state (memOnly)")
	}

	// Setup driver with initial target state
	tgt, ti, cleanup := driver.Setup(t, tgtFiles)
	defer cleanup()

	// Build MemSource with config + source files
	src := source.NewMemSource()
	cfgData := harness.ReadOrDie(cfgPath)
	memCfgPath := "/" + cfgFilename
	src.Files[memCfgPath] = cfgData
	for path, content := range srcFiles.Files {
		src.Files[path] = []byte(content)
	}

	// Run engine
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()

	run := func() error {
		cfg, err := engine.LoadConfig(ctx, em, memCfgPath, store, src)
		if err != nil {
			return err
		}

		resolved, err := engine.Resolve(cfg, "", "")
		if err != nil {
			return err
		}

		resolved.Target = ti
		resolved.Target.Config = ti.Config

		e, err := engine.NewWithTarget(ctx, src, resolved, em, tgt)
		if err != nil {
			return err
		}

		return e.Apply(ctx)
	}

	err := run()

	// Assert diagnostics first — they're recorded regardless of error/success
	if len(expect.Diagnostics) > 0 {
		gotDiags := rec.CollectDiagnosticIDs()
		if !stringSlicesEqual(gotDiags, expect.Diagnostics) {
			t.Fatalf("diagnostics: got %v, want %v", gotDiags, expect.Diagnostics)
		}
	}

	// Assert error expectation
	if expect.Error {
		if err == nil {
			t.Fatalf("expected error, got success\n%s", rec)
		}
		return
	}
	if err != nil {
		t.Fatalf("expected success, got error: %v\n%s", err, rec)
	}

	// Verify target state using driver
	driver.Verify(t, expect.Target)

	// Assert changed count
	gotChanged := rec.CountChangedOps()
	if gotChanged != expect.Changed {
		t.Errorf("changed count: got %d, want %d", gotChanged, expect.Changed)
	}
}

func loadE2EFiles(t *testing.T, path string) E2EFiles {
	t.Helper()
	data := harness.ReadOrDie(path)
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
	data := harness.ReadOrDie(path)
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
