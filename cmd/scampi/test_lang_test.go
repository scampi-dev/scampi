// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/source"
)

// nopDisplayer satisfies diagnostic.Displayer with no-op writes. Used
// to keep the runner happy in unit tests where we only care about
// the structured (passed, failed, err) return values, not what
// would be printed to the terminal.
type nopDisplayer struct {
	events []event.Event
}

func (d *nopDisplayer) EmitLegend() {}
func (d *nopDisplayer) Emit(e event.Event) {
	d.events = append(d.events, e)
}
func (d *nopDisplayer) Raise(r diagnostic.Raisable) { d.Emit(r.Diagnostic()) }
func (d *nopDisplayer) Interrupt()                  {}
func (d *nopDisplayer) Close()                      {}

var _ diagnostic.Displayer = (*nopDisplayer)(nil)

// writeTestFile writes content to a temp file and returns the path.
func writeTestFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func newTestEmitter() (diagnostic.Emitter, *nopDisplayer) {
	displ := &nopDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, displ)
	return em, displ
}

func TestRunLangTestFile_Passing(t *testing.T) {
	src := `module main

import "std"
import "std/posix"
import "std/test"
import "std/test/matchers"

let mock = test.target_in_memory(
  name = "mock",
  initial = test.InitialState {
    dirs = ["/etc"],
  },
  expect = test.ExpectedState {
    files = {
      "/etc/foo": matchers.has_substring("hello"),
    },
  },
)

std.deploy(name = "smoke", targets = [mock]) {
  posix.copy {
    dest = "/etc/foo"
    src = posix.source_inline { content = "hello world" }
    perm = "0644"
    owner = "root"
    group = "root"
  }
}
`
	path := writeTestFile(t, "smoke_test.scampi", src)
	em, displ := newTestEmitter()
	passed, failed, err := runLangTestFile(
		context.Background(),
		em,
		path,
		source.LocalPosixSource{},
	)
	if err != nil {
		for _, d := range displ.events {
			t.Logf("diag: %+v", d)
		}
		t.Fatalf("runLangTestFile: %v", err)
	}
	if passed != 1 || failed != 0 {
		t.Errorf("want 1 passed / 0 failed, got %d/%d (diags=%d)",
			passed, failed, len(displ.events))
	}
}

// Test that a _test.scampi file with the same module name as a sibling
// .scampi file can call non-pub functions from that sibling. This is
// the same-package test model (like Go). Regression test: the sibling
// loading guard used <= 1 which bailed when the entry file was a test
// file (excluded from readModuleDir).
func TestRunLangTestFile_SiblingModuleAccess(t *testing.T) {
	dir := t.TempDir()

	// Sibling module file with a non-pub function.
	apiSrc := `module mymod

import "std"
import "std/posix"

func write_foo(content: string) std.Step {
  return posix.copy {
    dest = "/etc/foo"
    src = posix.source_inline { content = content }
    perm = "0644"
    owner = "root"
    group = "root"
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "api.scampi"), []byte(apiSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test file: same module, calls sibling's non-pub function.
	testSrc := `module mymod

import "std"
import "std/posix"
import "std/test"
import "std/test/matchers"

let mock = test.target_in_memory(
  name = "mock",
  initial = test.InitialState {
    dirs = ["/etc"],
  },
  expect = test.ExpectedState {
    files = {
      "/etc/foo": matchers.has_exact_content("hello"),
    },
  },
)

std.deploy(name = "smoke", targets = [mock]) {
  write_foo(content = "hello")
}
`
	testPath := filepath.Join(dir, "api_test.scampi")
	if err := os.WriteFile(testPath, []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	em, displ := newTestEmitter()
	passed, failed, err := runLangTestFile(
		context.Background(),
		em,
		testPath,
		source.LocalPosixSource{},
	)
	if err != nil {
		for _, d := range displ.events {
			t.Logf("diag: %+v", d)
		}
		t.Fatalf("runLangTestFile: %v", err)
	}
	if passed != 1 || failed != 0 {
		t.Errorf("want 1 passed / 0 failed, got %d/%d", passed, failed)
	}
}

func TestRunLangTestFile_Failing(t *testing.T) {
	src := `module main

import "std"
import "std/posix"
import "std/test"
import "std/test/matchers"

let mock = test.target_in_memory(
  name = "mock",
  initial = test.InitialState {
    dirs = ["/etc"],
  },
  expect = test.ExpectedState {
    files = {
      "/etc/foo": matchers.has_exact_content("EXPECTED"),
    },
  },
)

std.deploy(name = "fail", targets = [mock]) {
  posix.copy {
    dest = "/etc/foo"
    src = posix.source_inline { content = "ACTUAL" }
    perm = "0644"
    owner = "root"
    group = "root"
  }
}
`
	path := writeTestFile(t, "fail_test.scampi", src)
	em, displ := newTestEmitter()
	passed, failed, err := runLangTestFile(
		context.Background(),
		em,
		path,
		source.LocalPosixSource{},
	)
	if err != nil {
		t.Fatalf("runLangTestFile: %v", err)
	}
	if passed != 0 || failed != 1 {
		t.Errorf("want 0 passed / 1 failed, got %d/%d", passed, failed)
	}
	if len(displ.events) == 0 {
		t.Errorf("expected at least one TestFail diagnostic")
	}
}
