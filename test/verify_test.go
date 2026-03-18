// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// Verify: copy step
// -----------------------------------------------------------------------------

func TestVerify_CopyPassesAndWrites(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		src = inline("hal9000 ALL=(ALL) NOPASSWD:ALL\n"),
		dest = "/sudoers-hal9000",
		perm = "0440",
		owner = "root",
		group = "root",
		verify = "visudo -cf %s",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(_ string) (target.CommandResult, error) {
		return target.CommandResult{ExitCode: 0}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	got := string(tgt.Files["/sudoers-hal9000"])
	want := "hal9000 ALL=(ALL) NOPASSWD:ALL\n"
	if got != want {
		t.Errorf("content = %q, want %q", got, want)
	}

	if tgt.Modes["/sudoers-hal9000"] != fs.FileMode(0o440) {
		t.Errorf("mode = %o, want 0440", tgt.Modes["/sudoers-hal9000"])
	}

	// Verify the command was called with a temp file path
	if len(tgt.Commands) == 0 {
		t.Fatal("expected verify command to be called")
	}
	if !strings.HasPrefix(tgt.Commands[0].Cmd, "visudo -cf /tmp/.scampi-") {
		t.Errorf("unexpected verify command: %s", tgt.Commands[0].Cmd)
	}
}

func TestVerify_CopyFailsAndLeavesDestUntouched(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		src = inline("INVALID SUDOERS\n"),
		dest = "/sudoers-bad",
		perm = "0440",
		owner = "root",
		group = "root",
		verify = "visudo -cf %s",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(_ string) (target.CommandResult, error) {
		return target.CommandResult{
			ExitCode: 1,
			Stderr:   "parse error in stdin",
		}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}

	if _, exists := tgt.Files["/sudoers-bad"]; exists {
		t.Error("destination should not have been written")
	}
}

func TestVerify_CopyMissingPlaceholder(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		src = inline("test\n"),
		dest = "/dest.txt",
		perm = "0644",
		owner = "root",
		group = "root",
		verify = "visudo -cf",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for missing placeholder")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

func TestVerify_CopyWithoutVerifyUnchanged(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		src = inline("plain content\n"),
		dest = "/dest.txt",
		perm = "0644",
		owner = "root",
		group = "root",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	got := string(tgt.Files["/dest.txt"])
	want := "plain content\n"
	if got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// Verify: template step
// -----------------------------------------------------------------------------

func TestVerify_TemplatePassesAndWrites(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	template(
		src = inline("server_name {{ .host }};"),
		dest = "/app.conf",
		perm = "0644",
		owner = "root",
		group = "root",
		data = {"values": {"host": "example.com"}},
		verify = "nginx -t -c %s",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(_ string) (target.CommandResult, error) {
		return target.CommandResult{ExitCode: 0}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	got := string(tgt.Files["/app.conf"])
	want := "server_name example.com;"
	if got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestVerify_TemplateFailsAndLeavesDestUntouched(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	template(
		src = inline("bad config {{ .x }}"),
		dest = "/bad.conf",
		perm = "0644",
		owner = "root",
		group = "root",
		data = {"values": {"x": "broken"}},
		verify = "nginx -t -c %s",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(_ string) (target.CommandResult, error) {
		return target.CommandResult{
			ExitCode: 1,
			Stderr:   "nginx: configuration file test failed",
		}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}

	if _, exists := tgt.Files["/bad.conf"]; exists {
		t.Error("destination should not have been written")
	}
}

func TestVerify_TemplateMissingPlaceholder(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	template(
		src = inline("test"),
		dest = "/dest.txt",
		perm = "0644",
		owner = "root",
		group = "root",
		verify = "nginx -t -c",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for missing placeholder")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// Verify: idempotency — verify is not re-run when content matches
// -----------------------------------------------------------------------------

func TestVerify_CopyIdempotentSkipsVerify(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		src = inline("already there\n"),
		dest = "/existing.txt",
		perm = "0644",
		owner = "root",
		group = "root",
		verify = "should-not-run %s",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	tgt.Files["/existing.txt"] = []byte("already there\n")
	tgt.Modes["/existing.txt"] = fs.FileMode(0o644)
	tgt.Owners["/existing.txt"] = target.Owner{User: "root", Group: "root"}

	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		t.Fatalf("verify command should not run on idempotent check, got: %s", cmd)
		return target.CommandResult{}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}
}

// Verify: temp file cleanup on failure
// -----------------------------------------------------------------------------

func TestVerify_TempFileCleanedUpOnFailure(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		src = inline("test content\n"),
		dest = "/verified.txt",
		perm = "0644",
		owner = "root",
		group = "root",
		verify = "false %s",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(_ string) (target.CommandResult, error) {
		return target.CommandResult{ExitCode: 1, Stderr: "fail"}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	_ = e.Apply(context.Background())

	for path := range tgt.Files {
		if strings.HasPrefix(path, "/tmp/.scampi-") {
			t.Errorf("temp file not cleaned up: %s", path)
		}
	}
}
