// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"scampi.dev/scampi/internal/engine"
)

type capturedEvent struct {
	Code engine.Code
	Ref  *engine.Ref
}

// captureEmitter records every emission and lets tests wait until a
// predicate over the collected events holds. notify is a coalescing
// channel: each Emit signals at most one pending waiter; the waiter
// re-evaluates the whole slice so coalesced signals never lose
// information.
type captureEmitter struct {
	mu     sync.Mutex
	events []capturedEvent
	notify chan struct{}
}

func (c *captureEmitter) Emit(_ context.Context, code engine.Code, ref *engine.Ref, _ ...any) {
	c.mu.Lock()
	c.events = append(c.events, capturedEvent{Code: code, Ref: ref})
	c.mu.Unlock()
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

// Events returns a snapshot of recorded events.
func (c *captureEmitter) Events() []capturedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.events)
}

// waitFor blocks until pred(events) is true or timeout expires.
func (c *captureEmitter) waitFor(pred func([]capturedEvent) bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if pred(c.Events()) {
			return true
		}
		wait := time.Until(deadline)
		if wait <= 0 {
			return false
		}
		select {
		case <-c.notify:
		case <-time.After(wait):
			return pred(c.Events())
		}
	}
}

// waitForCount waits until the captured events contain at least n
// instances of code. Returns true on match.
func (c *captureEmitter) waitForCount(code engine.Code, n int, timeout time.Duration) bool {
	return c.waitFor(func(events []capturedEvent) bool {
		count := 0
		for _, e := range events {
			if e.Code == code {
				count++
			}
		}
		return count >= n
	}, timeout)
}

func newCaptureLog() (engine.Log, *captureEmitter) {
	c := &captureEmitter{notify: make(chan struct{}, 1)}
	return engine.NewLog(c), c
}

// lifecycleOnly drops any log.* convenience events so tests can
// assert just on the stable lifecycle stream.
func lifecycleOnly(events []capturedEvent) []capturedEvent {
	out := events[:0]
	for _, e := range events {
		if engine.IsLogCode(e.Code) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Golden
// -----------------------------------------------------------------------------

type goldenExpected struct {
	Error  string
	Files  map[string]string
	Dirs   []string
	Absent []string
}

func TestGolden(t *testing.T) {
	cases, err := filepath.Glob("testdata/golden/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no golden cases under testdata/golden/")
	}
	for _, dir := range cases {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			runGoldenCase(t, dir)
		})
	}
}

func runGoldenCase(t *testing.T, caseDir string) {
	t.Helper()
	target := t.TempDir()
	cfg := t.TempDir()

	inputs, err := filepath.Glob(filepath.Join(caseDir, "*.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no *.hcl inputs under %s", caseDir)
	}
	for _, in := range inputs {
		src, err := os.ReadFile(in)
		if err != nil {
			t.Fatal(err)
		}
		out := strings.ReplaceAll(string(src), "{{TMP}}", target)
		dst := filepath.Join(cfg, filepath.Base(in))
		if err := os.WriteFile(dst, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	expectedYAML, err := os.ReadFile(filepath.Join(caseDir, "expected.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// {{TMP}} subs in both inputs and expected so ref-derived content
	// can name absolute tempdir paths.
	expectedYAML = []byte(strings.ReplaceAll(string(expectedYAML), "{{TMP}}", target))
	var want goldenExpected
	if err := yaml.Unmarshal(expectedYAML, &want); err != nil {
		t.Fatalf("expected.yaml: %v", err)
	}

	gotErr := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)

	switch want.Error {
	case "":
		if gotErr != nil {
			t.Fatalf("expected success, got: %v", gotErr)
		}
	case "snapshot":
		if !errors.Is(gotErr, engine.ErrSnapshotRejected) {
			t.Fatalf("expected engine.ErrSnapshotRejected, got: %v", gotErr)
		}
	case "apply":
		if !errors.Is(gotErr, engine.ErrApplyFailed) {
			t.Fatalf("expected engine.ErrApplyFailed, got: %v", gotErr)
		}
	default:
		t.Fatalf("unknown expected.error %q", want.Error)
	}

	for relPath, wantContent := range want.Files {
		got, err := os.ReadFile(filepath.Join(target, relPath))
		if err != nil {
			t.Errorf("file %s: %v", relPath, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("file %s: content = %q, want %q", relPath, got, wantContent)
		}
	}
	for _, relPath := range want.Dirs {
		info, err := os.Stat(filepath.Join(target, relPath))
		if err != nil {
			t.Errorf("dir %s: %v", relPath, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s: not a directory", relPath)
		}
	}
	for _, relPath := range want.Absent {
		if _, err := os.Stat(filepath.Join(target, relPath)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected absent: %s exists", relPath)
		}
	}
}

// Non-golden
// -----------------------------------------------------------------------------

// In-sync skip can't be asserted by post-state alone; the observable
// is mtime not advancing.
func TestApply_FileInSyncSkipsWrite(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, `
file "etc" {
  path    = "`+target+`"
  content = "hello"
}
`)
	if err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("mtime changed from %v to %v; expected no write when in sync",
			before.ModTime(), after.ModTime())
	}
}

func writeConfig(t *testing.T, hcl string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.hcl"), []byte(hcl), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Run mode
// -----------------------------------------------------------------------------

// The initial reconcile fires before the first ticker wait. Block
// on apply.success then cancel; verify after the engine returns.
func TestRun_AppliesOnceAtStart(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	cfg := writeConfig(t, `
file "x" {
  path    = "`+target+`"
  content = "hi"
}
`)
	log, capture := newCaptureLog()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx, cfg, 24*time.Hour, engine.NewInventory(), log) }()

	if !capture.waitForCount(engine.CodeApplySuccess, 1, 2*time.Second) {
		t.Fatal("timed out waiting for apply.success")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hi" {
		t.Errorf("content = %q, want %q", got, "hi")
	}
}

// Loop must pick up an in-flight config change. Synchronize on
// apply.success events (one before the mutation, two after) rather
// than polling the filesystem.
func TestRun_PicksUpChanges(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	cfg := t.TempDir()
	hclPath := filepath.Join(cfg, "main.hcl")
	writeHCL := func(content string) {
		t.Helper()
		hcl := `
file "x" {
  path    = "` + target + `"
  content = "` + content + `"
}
`
		if err := os.WriteFile(hclPath, []byte(hcl), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeHCL("first")

	log, capture := newCaptureLog()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx, cfg, 20*time.Millisecond, engine.NewInventory(), log) }()

	if !capture.waitForCount(engine.CodeApplySuccess, 1, 2*time.Second) {
		t.Fatal("timed out waiting for first apply.success")
	}
	writeHCL("second")
	if !capture.waitForCount(engine.CodeApplySuccess, 2, 2*time.Second) {
		t.Fatal("timed out waiting for second apply.success")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("content = %q, want %q", got, "second")
	}
}

// A successful apply emits the lifecycle codes in order: snapshot
// received, then apply.start + apply.success per resource that
// actually wrote (in-sync resources stay silent on the action log).
func TestApply_EmitsLifecycleEvents(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	cfg := writeConfig(t, `
file "x" {
  path    = "`+target+`"
  content = "hi"
}
`)
	log, capture := newCaptureLog()
	inv := engine.NewInventory()
	if err := engine.Apply(t.Context(), cfg, inv, log); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got := lifecycleOnly(capture.Events())
	want := []engine.Code{
		engine.CodeSnapshotReceived,
		engine.CodeApplyStart,
		engine.CodeApplySuccess,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lifecycle events, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Code != w {
			t.Errorf("event[%d] = %q, want %q", i, got[i].Code, w)
		}
	}
	// Cursor between the two applies so we assert only on what the
	// second one emits. Shared inventory: second apply finds file.x
	// in inventory and in sync, so it stays silent on the action log.
	cursor := len(capture.Events())
	if err := engine.Apply(t.Context(), cfg, inv, log); err != nil {
		t.Fatalf("Apply (second): %v", err)
	}
	got = lifecycleOnly(capture.Events()[cursor:])
	if len(got) != 1 || got[0].Code != engine.CodeSnapshotReceived {
		t.Errorf("second apply lifecycle events = %+v, want only snapshot.received", got)
	}
}

// Drift in observed state (the user mutated the target file out from
// under scampi) must converge back to declared on the next tick.
// Synchronize on apply.success: one when the engine plants the
// initial content, a second after we induce drift and the engine
// rewrites.
func TestRun_ConvergesDriftBack(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	cfg := writeConfig(t, `
file "x" {
  path    = "`+target+`"
  content = "desired"
}
`)

	log, capture := newCaptureLog()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx, cfg, 20*time.Millisecond, engine.NewInventory(), log) }()

	if !capture.waitForCount(engine.CodeApplySuccess, 1, 2*time.Second) {
		t.Fatal("timed out waiting for initial apply.success")
	}
	if err := os.WriteFile(target, []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !capture.waitForCount(engine.CodeApplySuccess, 2, 2*time.Second) {
		t.Fatal("timed out waiting for drift-fix apply.success")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "desired" {
		t.Errorf("content = %q, want %q after drift fix", got, "desired")
	}
}

// A persistently-failing resource must not get retried on every
// tick. Block on the first apply.failed, then wait inside the 1s
// backoff window to verify no second attempt fires. The sleep is
// bounded by what backoff means; the test still asserts the count.
func TestRun_BackoffSkipsPersistentFailure(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "missing-parent", "bad.txt")
	cfg := writeConfig(t, `
file "bad" {
  path    = "`+bad+`"
  content = "nope"
}
`)

	log, capture := newCaptureLog()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx, cfg, 20*time.Millisecond, engine.NewInventory(), log) }()

	if !capture.waitForCount(engine.CodeApplyFailed, 1, 2*time.Second) {
		t.Fatal("timed out waiting for first apply.failed")
	}
	// Sit inside the backoff window (first delay is 1s) to verify
	// no retry fires during it.
	time.Sleep(500 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	var attempts int
	for _, e := range capture.Events() {
		if e.Code == engine.CodeApplyStart {
			attempts++
		}
	}
	if attempts > 3 {
		t.Errorf("got %d apply.start attempts; backoff should cap this near 1", attempts)
	}
	if attempts == 0 {
		t.Errorf("got 0 apply.start attempts; the first try should have happened")
	}
}

// Inventory
// -----------------------------------------------------------------------------

func TestInventory_OrphansNone(t *testing.T) {
	inv := engine.NewInventory()
	inv.Add(engine.Ref{Kind: "file", Name: "a"}, map[string]string{"path": "/a"}, nil)
	declared := []engine.Resource{{Kind: "file", Name: "a"}}
	if got := inv.Orphans(declared); len(got) != 0 {
		t.Errorf("orphans = %+v, want empty", got)
	}
}

func TestInventory_OrphansOne(t *testing.T) {
	inv := engine.NewInventory()
	inv.Add(engine.Ref{Kind: "file", Name: "gone"}, map[string]string{"path": "/g"}, nil)
	got := inv.Orphans(nil)
	want := engine.Ref{Kind: "file", Name: "gone"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("orphans = %+v, want [%s]", got, want)
	}
}

func TestInventory_FoldApplyThenDestroy(t *testing.T) {
	inv := engine.NewInventory()
	ref := engine.Ref{Kind: "file", Name: "a"}
	inv.Fold(engine.CodeApplySuccess, ref, map[string]string{"path": "/x"})
	if !inv.Has(ref) {
		t.Fatalf("expected ref present after apply.success fold")
	}
	inv.Fold(engine.CodeDestroySuccess, ref, nil)
	if inv.Has(ref) {
		t.Errorf("expected ref absent after destroy.success fold")
	}
}

// Orphan handling
// -----------------------------------------------------------------------------

// Resources that were applied previously and then disappear from the
// declared snapshot must be destroyed on the next tick. Covers both
// file and dir Kinds so the dir destroy happy path is exercised end-
// to-end.
func TestRun_DestroysOrphan(t *testing.T) {
	tmp := t.TempDir()
	dirTarget := filepath.Join(tmp, "outdir")
	fileTarget := filepath.Join(dirTarget, "out.txt")
	cfg := t.TempDir()
	hclPath := filepath.Join(cfg, "main.hcl")
	writeHCL := func(content string) {
		t.Helper()
		if err := os.WriteFile(hclPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// file.x depends on dir.y by referencing dir.y.path. That edge is
	// what destroyOrder uses to remove the file before its parent dir
	// in a single tick.
	writeHCL(`
dir "y" {
  path = "` + dirTarget + `"
}
file "x" {
  path    = "${dir.y.path}/out.txt"
  content = "alive"
}
`)

	log, capture := newCaptureLog()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx, cfg, 20*time.Millisecond, engine.NewInventory(), log) }()

	if !capture.waitForCount(engine.CodeApplySuccess, 2, 2*time.Second) {
		t.Fatal("timed out waiting for initial apply.success x2")
	}
	writeHCL(`# empty intentionally
`)
	if !capture.waitForCount(engine.CodeDestroySuccess, 2, 2*time.Second) {
		t.Fatal("timed out waiting for destroy.success x2")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(fileTarget); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected %s absent after orphan destroy; got %v", fileTarget, err)
	}
	if _, err := os.Stat(dirTarget); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected %s absent after orphan destroy; got %v", dirTarget, err)
	}

	// file destroy must come before dir destroy (otherwise dir is non-
	// empty and the destroy fails on the first attempt, taking a second
	// tick to converge).
	fileRef := engine.Ref{Kind: "file", Name: "x"}
	dirRef := engine.Ref{Kind: "dir", Name: "y"}
	var fileAt, dirAt int
	for i, e := range lifecycleOnly(capture.Events()) {
		if e.Code != engine.CodeDestroySuccess || e.Ref == nil {
			continue
		}
		switch *e.Ref {
		case fileRef:
			fileAt = i
		case dirRef:
			dirAt = i
		}
	}
	if fileAt == 0 || dirAt == 0 {
		t.Fatalf("could not find destroy.success for both refs; fileAt=%d dirAt=%d", fileAt, dirAt)
	}
	if fileAt > dirAt {
		t.Errorf("expected file destroy before dir destroy; fileAt=%d dirAt=%d", fileAt, dirAt)
	}
}

// Action log durability
// -----------------------------------------------------------------------------

// The replay path is the durability story. Apply writes lifecycle
// events to the action log; a "restarted" process loads that log into
// a fresh inventory and continues. With the resource then removed from
// config, the second apply must treat it as an orphan and destroy it.
func TestActionLog_PersistsAcrossRuns(t *testing.T) {
	cfgDir := t.TempDir()
	targetDir := t.TempDir()
	alDir := t.TempDir()

	target := filepath.Join(targetDir, "out.txt")
	cfgPath := filepath.Join(cfgDir, "main.hcl")
	writeCfg := func(content string) {
		t.Helper()
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Run 1: declare the file, apply, persist to action log
	writeCfg(`
file "x" {
  path    = "` + target + `"
  content = "alive"
}
`)
	al1, err := engine.NewActionLog(alDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Apply(t.Context(), cfgDir, engine.NewInventory(), engine.NewLog(al1)); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := al1.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("after first apply, target should exist: %v", err)
	}

	// "Restart": rebuild the inventory from the action log alone
	inv, err := engine.LoadInventory(alDir)
	if err != nil {
		t.Fatalf("LoadInventory: %v", err)
	}
	if !inv.Has(engine.Ref{Kind: "file", Name: "x"}) {
		t.Fatalf("inventory should contain file.x after replay")
	}

	// Run 2: file removed from config; replay-built inventory drives destroy
	writeCfg(`# intentionally empty`)
	al2, err := engine.NewActionLog(alDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Apply(t.Context(), cfgDir, inv, engine.NewLog(al2)); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if err := al2.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected target absent after replay-driven destroy; got %v", err)
	}
}
