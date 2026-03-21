// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
)

const stateDir = "/var/lib/scampi/unarchive"

func destMarkerPath(dest string) string {
	h := sha256.Sum256([]byte(dest))
	return stateDir + "/" + hex.EncodeToString(h[:]) + ".sha256"
}

func archiveHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func makeTarGz(tb testing.TB, files map[string]string) []byte {
	tb.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			tb.Fatalf("tar WriteHeader: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			tb.Fatalf("tar Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatalf("tar Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		tb.Fatalf("gzip Close: %v", err)
	}
	return buf.Bytes()
}

// toolCommandFunc simulates tar/unzip being available and successful.
func toolCommandFunc(_ *target.MemTarget) func(string) (target.CommandResult, error) {
	return func(cmd string) (target.CommandResult, error) {
		switch {
		case strings.HasPrefix(cmd, "command -v "):
			return target.CommandResult{ExitCode: 0, Stdout: "/usr/bin/tar"}, nil
		case strings.HasPrefix(cmd, "mkdir -p "):
			return target.CommandResult{ExitCode: 0}, nil
		case strings.HasPrefix(cmd, "tar "):
			return target.CommandResult{ExitCode: 0}, nil
		case strings.HasPrefix(cmd, "chown "):
			return target.CommandResult{ExitCode: 0}, nil
		case strings.HasPrefix(cmd, "chmod "):
			return target.CommandResult{ExitCode: 0}, nil
		case strings.HasPrefix(cmd, "find "):
			return target.CommandResult{ExitCode: 0, Stdout: ""}, nil
		default:
			return target.CommandResult{ExitCode: 0}, nil
		}
	}
}

// Unarchive: basic extraction
// -----------------------------------------------------------------------------

func TestUnarchive_ExtractsAndWritesMarker(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"index.html": "<html>hello</html>",
	})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/site.tar.gz"),
		dest = "/var/www/site",
		depth = 0,
		desc = "extract site",
	),
])
`
	src := source.NewMemSource()
	src.Files["/site.tar.gz"] = archive

	tgt := target.NewMemTarget()
	tgt.CommandFunc = toolCommandFunc(tgt)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	marker := destMarkerPath("/var/www/site")
	markerData, ok := tgt.Files[marker]
	if !ok {
		t.Fatal("marker file not written")
	}

	want := archiveHash(archive)
	got := strings.TrimSpace(string(markerData))
	if got != want {
		t.Errorf("marker = %q, want %q", got, want)
	}
}

// Unarchive: idempotency
// -----------------------------------------------------------------------------

func TestUnarchive_IdempotentWhenMarkerMatches(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"index.html": "<html>hello</html>",
	})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/site.tar.gz"),
		dest = "/var/www/site",
		depth = 0,
	),
])
`
	src := source.NewMemSource()
	src.Files["/site.tar.gz"] = archive

	tgt := target.NewMemTarget()

	// Pre-populate marker with matching hash
	marker := destMarkerPath("/var/www/site")
	tgt.Files[marker] = []byte(archiveHash(archive) + "\n")

	commandCalled := false
	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		if strings.HasPrefix(cmd, "tar ") || strings.HasPrefix(cmd, "unzip ") {
			commandCalled = true
		}
		return target.CommandResult{ExitCode: 0, Stdout: ""}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	if commandCalled {
		t.Error("extraction command should not run when marker matches")
	}
}

// Unarchive: unsupported format
// -----------------------------------------------------------------------------

func TestUnarchive_UnsupportedFormat(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/archive.rar"),
		dest = "/output",
		depth = 0,
	),
])
`
	src := source.NewMemSource()
	src.Files["/archive.rar"] = []byte("not a real archive")
	tgt := target.NewMemTarget()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// Unarchive: missing source archive
// -----------------------------------------------------------------------------

func TestUnarchive_MissingSourceArchive(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/missing.tar.gz"),
		dest = "/output",
		depth = 0,
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.CommandFunc = toolCommandFunc(tgt)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for missing source")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// Unarchive: partial ownership
// -----------------------------------------------------------------------------

func TestUnarchive_PartialOwnership(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/site.tar.gz"),
		dest = "/var/www/site",
		owner = "www-data",
	),
])
`
	src := source.NewMemSource()
	src.Files["/site.tar.gz"] = makeTarGz(t, map[string]string{"f": "x"})
	tgt := target.NewMemTarget()
	tgt.CommandFunc = toolCommandFunc(tgt)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for owner without group")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// Unarchive: relative dest path
// -----------------------------------------------------------------------------

func TestUnarchive_RelativeDest(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/site.tar.gz"),
		dest = "relative/path",
	),
])
`
	src := source.NewMemSource()
	src.Files["/site.tar.gz"] = makeTarGz(t, map[string]string{"f": "x"})
	tgt := target.NewMemTarget()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for relative dest")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// Unarchive: with owner/group/perm
// -----------------------------------------------------------------------------

func TestUnarchive_WithOwnerGroupPerm(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"index.html": "<html>hello</html>",
	})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/site.tar.gz"),
		dest = "/var/www/site",
		depth = 0,
		owner = "www-data",
		group = "nogroup",
		perm = "0755",
	),
])
`
	src := source.NewMemSource()
	src.Files["/site.tar.gz"] = archive

	tgt := target.NewMemTarget()
	tgt.CommandFunc = toolCommandFunc(tgt)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	// ChownRecursive sets ownership on dest and all children
	if owner := tgt.Owners["/var/www/site"]; owner.User != "www-data" || owner.Group != "nogroup" {
		t.Errorf("dest owner = %s:%s, want www-data:nogroup", owner.User, owner.Group)
	}

	// ChmodRecursive sets mode on dest and all children
	if mode := tgt.Modes["/var/www/site"]; mode != 0o755 {
		t.Errorf("dest mode = %04o, want 0755", mode)
	}
}

// Unarchive: extraction failure
// -----------------------------------------------------------------------------

func TestUnarchive_ExtractionFailure(t *testing.T) {
	archive := makeTarGz(t, map[string]string{"f": "x"})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/site.tar.gz"),
		dest = "/var/www/site",
		depth = 0,
	),
])
`
	src := source.NewMemSource()
	src.Files["/site.tar.gz"] = archive

	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		if strings.HasPrefix(cmd, "command -v ") {
			return target.CommandResult{ExitCode: 0, Stdout: "/usr/bin/tar"}, nil
		}
		if strings.HasPrefix(cmd, "mkdir -p ") {
			return target.CommandResult{ExitCode: 0}, nil
		}
		if strings.HasPrefix(cmd, "tar ") {
			return target.CommandResult{ExitCode: 2, Stderr: "tar: corrupt archive"}, nil
		}
		return target.CommandResult{ExitCode: 0}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for extraction failure")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// Unarchive: on_change hook
// -----------------------------------------------------------------------------

func TestUnarchive_OnChangeTriggersHook(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"index.html": "<html>hello</html>",
	})

	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		unarchive(
			src = local("/site.tar.gz"),
			dest = "/var/www/site",
			depth = 0,
			on_change = ["restart-app"],
		),
	],
	hooks={
		"restart-app": service(
			name="app",
			state="restarted",
		),
	},
)
`
	src := source.NewMemSource()
	src.Files["/site.tar.gz"] = archive

	tgt := target.NewMemTarget()
	tgt.CommandFunc = toolCommandFunc(tgt)
	tgt.Services["app"] = true
	tgt.EnabledServices["app"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	if tgt.Restarts["app"] != 1 {
		t.Errorf("expected 1 restart, got %d", tgt.Restarts["app"])
	}
}

// Unarchive: Go-native fallback when tool missing
// -----------------------------------------------------------------------------

func TestUnarchive_GoNativeFallback(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"hello.txt": "world",
	})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/data.tar.gz"),
		dest = "/output",
		depth = 0,
	),
])
`
	src := source.NewMemSource()
	src.Files["/data.tar.gz"] = archive

	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		if strings.HasPrefix(cmd, "command -v ") {
			// Tool not available — force Go-native path
			return target.CommandResult{ExitCode: 1}, nil
		}
		if strings.HasPrefix(cmd, "mkdir -p ") {
			return target.CommandResult{ExitCode: 0}, nil
		}
		if strings.HasPrefix(cmd, "find ") {
			return target.CommandResult{ExitCode: 0, Stdout: ""}, nil
		}
		return target.CommandResult{ExitCode: 0}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	// Go-native extraction should have written the file directly
	got := string(tgt.Files["/output/hello.txt"])
	if got != "world" {
		t.Errorf("content = %q, want %q", got, "world")
	}

	// Marker should still be written
	marker := destMarkerPath("/output")
	if _, ok := tgt.Files[marker]; !ok {
		t.Error("marker file not written after Go-native extraction")
	}
}

// Unarchive: temp file cleanup on tool extraction failure
// -----------------------------------------------------------------------------

func TestUnarchive_TempFileCleanedUpOnFailure(t *testing.T) {
	archive := makeTarGz(t, map[string]string{"f": "x"})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/site.tar.gz"),
		dest = "/output",
		depth = 0,
	),
])
`
	src := source.NewMemSource()
	src.Files["/site.tar.gz"] = archive

	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		if strings.HasPrefix(cmd, "command -v ") {
			return target.CommandResult{ExitCode: 0, Stdout: "/usr/bin/tar"}, nil
		}
		if strings.HasPrefix(cmd, "mkdir -p ") {
			return target.CommandResult{ExitCode: 0}, nil
		}
		if strings.HasPrefix(cmd, "tar ") {
			return target.CommandResult{ExitCode: 2, Stderr: "corrupt"}, nil
		}
		return target.CommandResult{ExitCode: 0}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	_ = e.Apply(context.Background())

	for path := range tgt.Files {
		if strings.HasPrefix(path, "/tmp/.scampi-unarchive-") {
			t.Errorf("temp file not cleaned up: %s", path)
		}
	}
}

// Unarchive: default depth (0)
// -----------------------------------------------------------------------------

func TestUnarchive_DefaultDepthIsTopLevelOnly(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"data.txt": "hello",
	})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/data.tar.gz"),
		dest = "/output",
	),
])
`
	src := source.NewMemSource()
	src.Files["/data.tar.gz"] = archive

	tgt := target.NewMemTarget()

	findCalled := false
	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		if strings.HasPrefix(cmd, "find ") {
			findCalled = true
			return target.CommandResult{ExitCode: 0, Stdout: ""}, nil
		}
		return target.CommandResult{ExitCode: 0, Stdout: ""}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	// Default depth = 0 (top-level only), so find should NOT be called
	if findCalled {
		t.Error("expected find not to be called for nested archive detection (default depth = 0)")
	}
}

// noToolCommandFunc simulates tar not being available to force Go-native path.
func noToolCommandFunc() func(string) (target.CommandResult, error) {
	return func(cmd string) (target.CommandResult, error) {
		if strings.HasPrefix(cmd, "command -v ") {
			return target.CommandResult{ExitCode: 127}, nil
		}
		return target.CommandResult{ExitCode: 0, Stdout: ""}, nil
	}
}

func makeTarXz(tb testing.TB, files map[string]string) []byte {
	tb.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		tb.Fatalf("xz NewWriter: %v", err)
	}
	tw := tar.NewWriter(xw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			tb.Fatalf("tar WriteHeader: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			tb.Fatalf("tar Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatalf("tar Close: %v", err)
	}
	if err := xw.Close(); err != nil {
		tb.Fatalf("xz Close: %v", err)
	}
	return buf.Bytes()
}

func makeTarZst(tb testing.TB, files map[string]string) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		tb.Fatalf("zstd NewWriter: %v", err)
	}
	tw := tar.NewWriter(zw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			tb.Fatalf("tar WriteHeader: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			tb.Fatalf("tar Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatalf("tar Close: %v", err)
	}
	if err := zw.Close(); err != nil {
		tb.Fatalf("zstd Close: %v", err)
	}
	return buf.Bytes()
}

func makeTar(tb testing.TB, files map[string]string) []byte {
	tb.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			tb.Fatalf("tar WriteHeader: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			tb.Fatalf("tar Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatalf("tar Close: %v", err)
	}
	return buf.Bytes()
}

func makeZip(tb testing.TB, files map[string]string) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			tb.Fatalf("zip Create: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			tb.Fatalf("zip Write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		tb.Fatalf("zip Close: %v", err)
	}
	return buf.Bytes()
}

// Unarchive: Go-native tar.xz extraction
// -----------------------------------------------------------------------------

func TestUnarchive_TarXz(t *testing.T) {
	archive := makeTarXz(t, map[string]string{
		"hello.txt": "xz world",
	})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/data.tar.xz"),
		dest = "/output",
		depth = 0,
	),
])
`
	src := source.NewMemSource()
	src.Files["/data.tar.xz"] = archive

	tgt := target.NewMemTarget()
	tgt.CommandFunc = noToolCommandFunc()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	got := string(tgt.Files["/output/hello.txt"])
	if got != "xz world" {
		t.Errorf("content = %q, want %q", got, "xz world")
	}

	if _, ok := tgt.Files[destMarkerPath("/output")]; !ok {
		t.Error("marker file not written after Go-native xz extraction")
	}
}

// Unarchive: Go-native tar.zst extraction
// -----------------------------------------------------------------------------

func TestUnarchive_TarZst(t *testing.T) {
	archive := makeTarZst(t, map[string]string{
		"hello.txt": "zstd world",
	})

	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	unarchive(
		src = local("/data.tar.zst"),
		dest = "/output",
		depth = 0,
	),
])
`
	src := source.NewMemSource()
	src.Files["/data.tar.zst"] = archive

	tgt := target.NewMemTarget()
	tgt.CommandFunc = noToolCommandFunc()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v\n%s", err, rec)
	}

	got := string(tgt.Files["/output/hello.txt"])
	if got != "zstd world" {
		t.Errorf("content = %q, want %q", got, "zstd world")
	}

	if _, ok := tgt.Files[destMarkerPath("/output")]; !ok {
		t.Error("marker file not written after Go-native zstd extraction")
	}
}
