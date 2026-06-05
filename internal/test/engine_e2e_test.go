// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"errors"
	"fmt"
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

func (*captureEmitter) Err() error { return nil }

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

func (c *captureEmitter) has(code engine.Code) bool {
	for _, e := range c.Events() {
		if e.Code == code {
			return true
		}
	}
	return false
}

func (c *captureEmitter) codes() []engine.Code {
	events := c.Events()
	out := make([]engine.Code, len(events))
	for i, e := range events {
		out[i] = e.Code
	}
	return out
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
		if e.Code.IsLifecycle() {
			out = append(out, e)
		}
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
func TestApply_AdoptMatchingSkipsWrite(t *testing.T) {
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
  adopt   = true
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
		t.Errorf("mtime changed from %v to %v; expected no write on matching adopt",
			before.ModTime(), after.ModTime())
	}
}

func TestApply_HaltsMatchingWithoutAdopt(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, `
file "etc" {
  path    = "`+target+`"
  content = "hello"
}
`)
	log, capture := newCaptureLog()
	inv := engine.NewInventory()
	if err := engine.Apply(t.Context(), cfg, inv, log); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !capture.has(engine.CodeApplyHalted) {
		t.Errorf("expected apply.halted; got codes %v", capture.codes())
	}
	if inv.Has(engine.Ref{Kind: "file", Name: "etc"}) {
		t.Error("expected resource NOT in inventory after halt")
	}
}

func TestApply_HaltsDivergingWithoutAdopt(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	if err := os.WriteFile(target, []byte("on-disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, `
file "etc" {
  path    = "`+target+`"
  content = "desired"
}
`)
	log, capture := newCaptureLog()
	inv := engine.NewInventory()
	if err := engine.Apply(t.Context(), cfg, inv, log); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !capture.has(engine.CodeApplyHalted) {
		t.Errorf("expected apply.halted; got codes %v", capture.codes())
	}
	if inv.Has(engine.Ref{Kind: "file", Name: "etc"}) {
		t.Error("expected resource NOT in inventory after halt")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "on-disk" {
		t.Errorf("file content = %q, want %q; halt must not write",
			got, "on-disk")
	}
}

func TestApply_AdoptTakesOverDiverging(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.txt")
	if err := os.WriteFile(target, []byte("on-disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, `
file "etc" {
  path    = "`+target+`"
  content = "desired"
  adopt   = true
}
`)
	inv := engine.NewInventory()
	if err := engine.Apply(t.Context(), cfg, inv, engine.Discard); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "desired" {
		t.Errorf("file content = %q, want %q after adopt takeover",
			got, "desired")
	}
	if !inv.Has(engine.Ref{Kind: "file", Name: "etc"}) {
		t.Error("expected resource in inventory after adopt takeover")
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
	inv.Add(engine.Ref{Kind: "file", Name: "a"}, engine.Attrs{"path": engine.StringValue("/a")}, nil)
	declared := []engine.Resource{{Kind: "file", Name: "a"}}
	if got := inv.Orphans(declared); len(got) != 0 {
		t.Errorf("orphans = %+v, want empty", got)
	}
}

func TestInventory_OrphansOne(t *testing.T) {
	inv := engine.NewInventory()
	inv.Add(engine.Ref{Kind: "file", Name: "gone"}, engine.Attrs{"path": engine.StringValue("/g")}, nil)
	got := inv.Orphans(nil)
	want := engine.Ref{Kind: "file", Name: "gone"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("orphans = %+v, want [%s]", got, want)
	}
}

func TestInventory_FoldApplyThenDestroy(t *testing.T) {
	inv := engine.NewInventory()
	ref := engine.Ref{Kind: "file", Name: "a"}
	inv.Fold(engine.CodeApplySuccess, ref, engine.Attrs{"path": engine.StringValue("/x")})
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

// A resource keeping its ref but changing its identity attrs (e.g.
// dir.tmp's path moves) must destroy the prior live resource and
// create the new one. Otherwise the old live state lingers untracked.
func TestApply_IdentityDriftOnSameRefDestroysOld(t *testing.T) {
	tmp := t.TempDir()
	oldPath := filepath.Join(tmp, "old")
	newPath := filepath.Join(tmp, "new")
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "main.hcl")
	writeCfg := func(content string) {
		t.Helper()
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Apply 1: dir.tmp at oldPath.
	writeCfg(`
dir "tmp" {
  path = "` + oldPath + `"
}
`)
	inv := engine.NewInventory()
	if err := engine.Apply(t.Context(), cfgDir, inv, engine.Discard); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old dir should exist after first apply: %v", err)
	}

	// Apply 2: dir.tmp moves to newPath while keeping the same ref.
	writeCfg(`
dir "tmp" {
  path = "` + newPath + `"
}
`)
	if err := engine.Apply(t.Context(), cfgDir, inv, engine.Discard); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new dir should exist after second apply: %v", err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected old dir destroyed after identity drift; got %v", err)
	}
}

// Renaming a resource (changing its HCL block label while keeping
// its identity attrs the same) must move the inventory entry rather
// than destroy + create. Otherwise destroy on a non-empty dir fails
// and even where it succeeds the live state churns needlessly.
func TestApply_RenameSwapsInventoryNotDestroyCreate(t *testing.T) {
	tmp := t.TempDir()
	dirPath := filepath.Join(tmp, "shared")
	filePath := filepath.Join(dirPath, "yolo")
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "main.hcl")
	writeCfg := func(content string) {
		t.Helper()
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// rev 1: dir "tmp" + file "yolo" inside it.
	writeCfg(`
dir "tmp" {
  path = "` + dirPath + `"
}
file "yolo" {
  path    = "${dir.tmp.path}/yolo"
  content = "hi"
}
`)
	inv := engine.NewInventory()
	if err := engine.Apply(t.Context(), cfgDir, inv, engine.Discard); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if !inv.Has(engine.Ref{Kind: "dir", Name: "tmp"}) {
		t.Fatalf("inventory should have dir.tmp after first apply")
	}

	// rev 2: rename dir.tmp -> dir.tmp2 (same path). file.yolo references
	// the renamed dir but otherwise unchanged.
	writeCfg(`
dir "tmp2" {
  path = "` + dirPath + `"
}
file "yolo" {
  path    = "${dir.tmp2.path}/yolo"
  content = "hi"
}
`)
	if err := engine.Apply(t.Context(), cfgDir, inv, engine.Discard); err != nil {
		t.Fatalf("second apply (rename): %v", err)
	}

	// Inventory moved.
	if inv.Has(engine.Ref{Kind: "dir", Name: "tmp"}) {
		t.Error("dir.tmp should no longer be in inventory after rename")
	}
	if !inv.Has(engine.Ref{Kind: "dir", Name: "tmp2"}) {
		t.Error("dir.tmp2 should be in inventory after rename")
	}
	// Live state preserved: the dir and the file it contains still exist.
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		t.Errorf("shared dir should still exist: %v", err)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("file inside should still exist: %v", err)
	}
}

// brokenEmitter reports a sticky failure via Err. Used to verify the
// engine aborts a reconcile pass on first action-log breakage.
type brokenEmitter struct{ err error }

func (b *brokenEmitter) Emit(context.Context, engine.Code, *engine.Ref, ...any) {}
func (b *brokenEmitter) Err() error                                             { return b.err }

// Apply must surface and propagate a sticky action-log failure rather
// than acting blind. The wrapped emitter error must show up under
// ErrApplyFailed so main exits cleanly.
func TestApply_AbortsOnActionLogFailure(t *testing.T) {
	cfg := t.TempDir()
	tmp := t.TempDir()
	target := filepath.Join(tmp, "f.txt")
	if err := os.WriteFile(filepath.Join(cfg, "main.hcl"), []byte(`
file "x" {
  path    = "`+target+`"
  content = "alive"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	bad := &brokenEmitter{err: errors.New("simulated action log failure")}
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.NewLog(bad))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, engine.ErrApplyFailed) {
		t.Errorf("expected ErrApplyFailed; got %v", err)
	}
	if !strings.Contains(err.Error(), "simulated action log failure") {
		t.Errorf("expected wrapped emitter error; got %v", err)
	}
}

// Plan
// -----------------------------------------------------------------------------

func TestMakePlan_Categories(t *testing.T) {
	tmp := t.TempDir()
	match := filepath.Join(tmp, "match")
	gone := filepath.Join(tmp, "gone")
	drift := filepath.Join(tmp, "drift")
	halt := filepath.Join(tmp, "halt")
	newPath := filepath.Join(tmp, "new")

	// Pre-existing state that the first apply will adopt.
	if err := os.WriteFile(match, []byte("right"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(drift, []byte("right"), 0o644); err != nil {
		t.Fatal(err)
	}

	// First apply: populate the inventory with match, gone, drift.
	cfg1 := writeConfig(t, fmt.Sprintf(`
file "match" {
  path    = %q
  content = "right"
  adopt   = true
}
file "drift" {
  path    = %q
  content = "right"
  adopt   = true
}
file "gone" {
  path    = %q
  content = "x"
}
`, match, drift, gone))
	inv := engine.NewInventory()
	if err := engine.Apply(t.Context(), cfg1, inv, engine.Discard); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Provoke each category for the plan:
	//   drift   -> on-disk content now differs from desired      (update)
	//   halt    -> exists on disk, NOT in inv, no adopt          (halt)
	//   new     -> not on disk, not in inv                       (create)
	//   match   -> on disk, in inv, in-sync                      (in-sync)
	//   gone    -> in inv, dropped from new config               (destroy)
	if err := os.WriteFile(drift, []byte("drifted-again"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(halt, []byte("preexisting"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg2 := writeConfig(t, fmt.Sprintf(`
file "match" {
  path    = %q
  content = "right"
  adopt   = true
}
file "drift" {
  path    = %q
  content = "right"
  adopt   = true
}
file "new" {
  path    = %q
  content = "y"
}
file "halt" {
  path    = %q
  content = "preexisting"
}
`, match, drift, newPath, halt))

	p, err := engine.MakePlan(t.Context(), cfg2, inv, engine.Discard)
	if err != nil {
		t.Fatalf("MakePlan: %v", err)
	}

	want := map[string][]string{
		"create":  {"file.new"},
		"update":  {"file.drift"},
		"halt":    {"file.halt"},
		"destroy": {"file.gone"},
		"in-sync": {"file.match"},
	}
	got := map[string][]string{
		"create":  refNames(p.Create),
		"update":  refNames(p.Update),
		"halt":    refNames(p.Halt),
		"destroy": refNames(p.Destroy),
		"in-sync": refNames(p.InSync),
	}
	for k, w := range want {
		if !slices.Equal(got[k], w) {
			t.Errorf("%s: got %v, want %v", k, got[k], w)
		}
	}
}

func refNames(refs []engine.Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.String()
	}
	return out
}

// Action log replay modes
// -----------------------------------------------------------------------------

func TestLoadInventory_LenientToleratesPartialTail(t *testing.T) {
	dir := t.TempDir()
	seg := filepath.Join(dir, "0001.jsonl")
	good := `{"ts":"x","code":"apply.success","ref":"file.a","path":"/p","deps":""}` + "\n"
	partial := `{"ts":"x","code":"apply.suc`
	if err := os.WriteFile(seg, []byte(good+partial), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.LoadInventory(dir); err == nil {
		t.Error("strict load should reject trailing partial line")
	}
	inv, err := engine.LoadInventoryLenient(dir)
	if err != nil {
		t.Fatalf("lenient load failed: %v", err)
	}
	if !inv.Has(engine.Ref{Kind: "file", Name: "a"}) {
		t.Error("lenient load should still fold complete lines")
	}
}

func TestLoadInventory_StrictRejectsMidStreamCorruption(t *testing.T) {
	dir := t.TempDir()
	seg := filepath.Join(dir, "0001.jsonl")
	good := `{"ts":"x","code":"apply.success","ref":"file.a","path":"/p","deps":""}` + "\n"
	bad := `not-json` + "\n"
	more := `{"ts":"x","code":"apply.success","ref":"file.b","path":"/q","deps":""}` + "\n"
	if err := os.WriteFile(seg, []byte(good+bad+more), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.LoadInventory(dir); err == nil {
		t.Error("strict load should reject corrupted mid-stream line")
	}
	if _, err := engine.LoadInventoryLenient(dir); err == nil {
		t.Error("lenient load should still reject corrupted mid-stream line (not the trailing partial)")
	}
}

// Path validation
// -----------------------------------------------------------------------------

// Duplicate detection
// -----------------------------------------------------------------------------

func TestSnapshot_DuplicateRef(t *testing.T) {
	tmp := t.TempDir()
	cfg := writeConfig(t, fmt.Sprintf(`
file "dup" {
  path    = "%s/a"
  content = "1"
}
file "dup" {
  path    = "%s/b"
  content = "2"
}
`, tmp, tmp))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Errorf("expected snapshot rejection; got %v", err)
	}
}

func TestSnapshot_DuplicateIdentity(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "x")
	cfg := writeConfig(t, fmt.Sprintf(`
file "a" {
  path    = "%s"
  content = "from-a"
}
file "b" {
  path    = "%s"
  content = "from-b"
}
`, path, path))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Errorf("expected snapshot rejection; got %v", err)
	}
}

// Identity collision still fires when the colliding attr arrives
// through a ref expression rather than a literal duplicate.
func TestSnapshot_DuplicateIdentityViaRef(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "shared")
	cfg := writeConfig(t, fmt.Sprintf(`
dir "a" {
  path = "%s"
}
dir "b" {
  path = dir.a.path
}
`, path))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Errorf("expected snapshot rejection; got %v", err)
	}
}

// Schema validation
// -----------------------------------------------------------------------------

func TestSnapshot_SchemaMissingRequired(t *testing.T) {
	tmp := t.TempDir()
	cfg := writeConfig(t, fmt.Sprintf(`
file "x" {
  path = "%s/x"
}
`, tmp))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Fatalf("expected snapshot rejection; got %v", err)
	}
	if !strings.Contains(err.Error(), `file.x: missing required attr "content"`) {
		t.Errorf("expected missing-required message; got %v", err)
	}
}

func TestSnapshot_SchemaUnknownAttr(t *testing.T) {
	tmp := t.TempDir()
	cfg := writeConfig(t, fmt.Sprintf(`
file "x" {
  path    = "%s/x"
  content = "y"
  contnet = "z"
}
`, tmp))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Fatalf("expected snapshot rejection; got %v", err)
	}
	if !strings.Contains(err.Error(), `file.x: unknown attr "contnet"`) {
		t.Errorf("expected unknown-attr message; got %v", err)
	}
}

// Typos with a ref-bearing RHS sit in pending pre-resolve. They must
// still be flagged before any resolution work runs.
func TestSnapshot_SchemaUnknownAttrInPending(t *testing.T) {
	tmp := t.TempDir()
	cfg := writeConfig(t, fmt.Sprintf(`
dir "d" {
  path = "%s/d"
}
file "x" {
  path    = "%s/x"
  content = "y"
  contnet = dir.d.path
}
`, tmp, tmp))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Fatalf("expected snapshot rejection; got %v", err)
	}
	if !strings.Contains(err.Error(), `file.x: unknown attr "contnet"`) {
		t.Errorf("expected unknown-attr message; got %v", err)
	}
}

func TestSnapshot_SchemaAggregatesAcrossResources(t *testing.T) {
	tmp := t.TempDir()
	cfg := writeConfig(t, fmt.Sprintf(`
file "a" {
  path = "%s/a"
}
file "b" {
  path    = "%s/b"
  content = "y"
  bogus   = "z"
}
`, tmp, tmp))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Fatalf("expected snapshot rejection; got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, `file.a: missing required attr "content"`) {
		t.Errorf("missing file.a error in %v", err)
	}
	if !strings.Contains(msg, `file.b: unknown attr "bogus"`) {
		t.Errorf("missing file.b error in %v", err)
	}
}

// Typed schema
// -----------------------------------------------------------------------------

func TestSnapshot_BoolAttrRejectsString(t *testing.T) {
	tmp := t.TempDir()
	cfg := writeConfig(t, fmt.Sprintf(`
file "x" {
  path    = "%s/x"
  content = "y"
  adopt   = "true"
}
`, tmp))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Fatalf("expected snapshot rejection; got %v", err)
	}
	if !strings.Contains(err.Error(), `attr "adopt"`) || !strings.Contains(err.Error(), "bool") {
		t.Errorf("expected bool-type error mentioning attr; got %v", err)
	}
}

func TestSnapshot_StringAttrRejectsBool(t *testing.T) {
	cfg := writeConfig(t, `
file "x" {
  path    = true
  content = "y"
}
`)
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Fatalf("expected snapshot rejection; got %v", err)
	}
	if !strings.Contains(err.Error(), `attr "path"`) || !strings.Contains(err.Error(), "string") {
		t.Errorf("expected string-type error mentioning attr; got %v", err)
	}
}

// Refs evaluate to strings today. A ref-bearing RHS on a bool attr
// (or any non-string) must reject at typecheck before resolve runs.
func TestSnapshot_RefRejectedOnBoolAttr(t *testing.T) {
	tmp := t.TempDir()
	cfg := writeConfig(t, fmt.Sprintf(`
file "src" {
  path    = "%s/src"
  content = "hi"
}
file "tgt" {
  path    = "%s/tgt"
  content = "hi"
  adopt   = file.src.path
}
`, tmp, tmp))
	err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
	if !errors.Is(err, engine.ErrSnapshotRejected) {
		t.Fatalf("expected snapshot rejection; got %v", err)
	}
	if !strings.Contains(err.Error(), `attr "adopt"`) {
		t.Errorf("expected error mentioning adopt; got %v", err)
	}
}

func TestSnapshot_PathValidation(t *testing.T) {
	tmp := t.TempDir()
	good := filepath.Join(tmp, "ok.txt")

	cases := []struct {
		name   string
		path   string
		reject bool
	}{
		{"absolute", good, false},
		{"relative", "etc/foo", true},
		{"traversal", "/etc/../tmp/foo", true},
		{"double-slash", "/etc//foo", true},
		{"trailing-slash", "/etc/foo/", true},
		{"shell-home", "$HOME/foo", true},
		{"tilde", "~/foo", true},
		{"root-only", "/", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := writeConfig(t, fmt.Sprintf(`
file "x" {
  path    = %q
  content = "y"
}
`, c.path))
			err := engine.Apply(t.Context(), cfg, engine.NewInventory(), engine.Discard)
			rejected := errors.Is(err, engine.ErrSnapshotRejected)
			if rejected != c.reject {
				t.Errorf("path %q: rejected=%v want %v (err=%v)", c.path, rejected, c.reject, err)
			}
		})
	}
}
