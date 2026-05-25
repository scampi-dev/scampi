// SPDX-License-Identifier: GPL-3.0-only

package mod_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"scampi.dev/scampi/mod"
	"scampi.dev/scampi/source"
)

// statFailingSource wraps MemSource and returns the configured error from Stat.
// Used to exercise the Stat-failure branch of mod.Init without touching the FS.
type statFailingSource struct {
	*source.MemSource
	statErr error
}

func (s *statFailingSource) Stat(_ context.Context, _ string) (source.FileMeta, error) {
	return source.FileMeta{}, s.statErr
}

func TestInit_WritesScampiModWhenMissing(t *testing.T) {
	src := source.NewMemSource()
	const path = "github.com/me/foo"

	if err := mod.Init(context.Background(), src, "/proj", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := src.ReadFile(context.Background(), "/proj/scampi.mod")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "module " + path + "\n"
	if string(data) != want {
		t.Errorf("scampi.mod content = %q, want %q", string(data), want)
	}
}

func TestInit_ErrorsWhenScampiModExists(t *testing.T) {
	src := source.NewMemSource()
	if err := src.WriteFile(context.Background(), "/proj/scampi.mod", []byte("module old\n")); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	err := mod.Init(context.Background(), src, "/proj", "github.com/me/foo")
	if err == nil {
		t.Fatal("expected error when scampi.mod exists, got nil")
	}

	var initErr *mod.InitError
	if !errors.As(err, &initErr) {
		t.Fatalf("expected *mod.InitError, got %T (%v)", err, err)
	}
	if !strings.Contains(initErr.Detail, "already exists") {
		t.Errorf("Detail = %q, want substring %q", initErr.Detail, "already exists")
	}

	// Existing file must not be overwritten.
	data, _ := src.ReadFile(context.Background(), "/proj/scampi.mod")
	if string(data) != "module old\n" {
		t.Errorf("existing scampi.mod was overwritten: got %q", string(data))
	}
}

func TestInit_ReturnsInitStatErrorOnStatFailure(t *testing.T) {
	statErr := errors.New("permission denied")
	src := &statFailingSource{
		MemSource: source.NewMemSource(),
		statErr:   statErr,
	}

	err := mod.Init(context.Background(), src, "/proj", "github.com/me/foo")
	if err == nil {
		t.Fatal("expected error when Stat fails, got nil")
	}

	var statE *mod.InitStatError
	if !errors.As(err, &statE) {
		t.Fatalf("expected *mod.InitStatError, got %T (%v)", err, err)
	}
	if statE.Path != "/proj/scampi.mod" {
		t.Errorf("Path = %q, want %q", statE.Path, "/proj/scampi.mod")
	}
	if !errors.Is(err, statErr) {
		t.Error("InitStatError must wrap the original Stat error")
	}
}

func TestInit_RejectsInvalidModulePath(t *testing.T) {
	src := source.NewMemSource()
	err := mod.Init(context.Background(), src, "/proj", "not-a-valid-path")
	if err == nil {
		t.Fatal("expected error for invalid module path, got nil")
	}
	var initErr *mod.InitError
	if !errors.As(err, &initErr) {
		t.Fatalf("expected *mod.InitError, got %T", err)
	}
	if !strings.Contains(initErr.Detail, "invalid module path") {
		t.Errorf("Detail = %q, want substring %q", initErr.Detail, "invalid module path")
	}
}
