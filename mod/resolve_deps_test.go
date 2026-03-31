// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"errors"
	"testing"

	"scampi.dev/scampi/source"
)

func modContent(module string, deps ...string) []byte {
	s := "module " + module + "\n"
	if len(deps) > 0 {
		s += "\nrequire (\n"
		for _, d := range deps {
			s += "\t" + d + "\n"
		}
		s += ")\n"
	}
	return []byte(s)
}

func TestResolveDeps_NoTransitive(t *testing.T) {
	src := source.NewMemSource()
	ctx := context.Background()
	cacheDir := "/cache"

	// A has no scampi.mod in cache — leaf module.
	direct := []Dependency{
		{Path: "codeberg.org/user/a", Version: "v1.0.0"},
	}

	got, err := ResolveDeps(ctx, src, direct, cacheDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d deps, want 1", len(got))
	}
	if got[0].Path != "codeberg.org/user/a" || got[0].Version != "v1.0.0" {
		t.Errorf("got %s@%s, want codeberg.org/user/a@v1.0.0", got[0].Path, got[0].Version)
	}
	if got[0].Indirect {
		t.Error("direct dep should not be indirect")
	}
}

func TestResolveDeps_SimpleChain(t *testing.T) {
	src := source.NewMemSource()
	ctx := context.Background()
	cacheDir := "/cache"

	// A@v1.0.0 → B@v1.0.0 → C@v1.0.0
	src.Files["/cache/codeberg.org/user/a@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/a", "codeberg.org/user/b v1.0.0")
	src.Files["/cache/codeberg.org/user/b@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/b", "codeberg.org/user/c v1.0.0")

	direct := []Dependency{
		{Path: "codeberg.org/user/a", Version: "v1.0.0"},
	}

	got, err := ResolveDeps(ctx, src, direct, cacheDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d deps, want 3", len(got))
	}

	// Sorted by path: a, b, c
	assertDep(t, got[0], "codeberg.org/user/a", "v1.0.0", false)
	assertDep(t, got[1], "codeberg.org/user/b", "v1.0.0", true)
	assertDep(t, got[2], "codeberg.org/user/c", "v1.0.0", true)
}

func TestResolveDeps_Diamond(t *testing.T) {
	src := source.NewMemSource()
	ctx := context.Background()
	cacheDir := "/cache"

	// A → B@v1.0.0, C@v1.0.0
	// B@v1.0.0 → D@v1.0.0
	// C@v1.0.0 → D@v2.0.0   ← higher wins
	src.Files["/cache/codeberg.org/user/a@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/a",
			"codeberg.org/user/b v1.0.0",
			"codeberg.org/user/c v1.0.0",
		)
	src.Files["/cache/codeberg.org/user/b@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/b", "codeberg.org/user/d v1.0.0")
	src.Files["/cache/codeberg.org/user/c@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/c", "codeberg.org/user/d v2.0.0")

	direct := []Dependency{
		{Path: "codeberg.org/user/a", Version: "v1.0.0"},
	}

	got, err := ResolveDeps(ctx, src, direct, cacheDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find D in the results.
	var d *Dependency
	for i := range got {
		if got[i].Path == "codeberg.org/user/d" {
			d = &got[i]
			break
		}
	}
	if d == nil {
		t.Fatal("D not found in resolved deps")
	}
	if d.Version != "v2.0.0" {
		t.Errorf("D version = %s, want v2.0.0 (highest wins)", d.Version)
	}
	if !d.Indirect {
		t.Error("D should be indirect")
	}
}

func TestResolveDeps_Cycle(t *testing.T) {
	src := source.NewMemSource()
	ctx := context.Background()
	cacheDir := "/cache"

	// A → B, B → A
	src.Files["/cache/codeberg.org/user/a@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/a", "codeberg.org/user/b v1.0.0")
	src.Files["/cache/codeberg.org/user/b@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/b", "codeberg.org/user/a v1.0.0")

	direct := []Dependency{
		{Path: "codeberg.org/user/a", Version: "v1.0.0"},
	}

	_, err := ResolveDeps(ctx, src, direct, cacheDir)
	if err == nil {
		t.Fatal("expected CycleError, got nil")
	}

	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("expected CycleError, got %T: %v", err, err)
	}
	if len(ce.Chain) < 2 {
		t.Errorf("cycle chain too short: %v", ce.Chain)
	}
}

func TestResolveDeps_ModuleNoScampiMod(t *testing.T) {
	src := source.NewMemSource()
	ctx := context.Background()
	cacheDir := "/cache"

	// Module exists in cache but has no scampi.mod — leaf node.
	direct := []Dependency{
		{Path: "codeberg.org/user/leaf", Version: "v1.0.0"},
	}

	got, err := ResolveDeps(ctx, src, direct, cacheDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d deps, want 1", len(got))
	}
	assertDep(t, got[0], "codeberg.org/user/leaf", "v1.0.0", false)
}

func TestResolveDeps_LocalDepSkipped(t *testing.T) {
	src := source.NewMemSource()
	ctx := context.Background()
	cacheDir := "/cache"

	// Local dep should pass through without resolution.
	// Also add a remote dep with transitive to prove they work side by side.
	src.Files["/cache/codeberg.org/user/remote@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/remote", "codeberg.org/user/trans v1.0.0")

	direct := []Dependency{
		{Path: "my-local", Version: "./libs/local"},
		{Path: "codeberg.org/user/remote", Version: "v1.0.0"},
	}

	got, err := ResolveDeps(ctx, src, direct, cacheDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// local + remote + trans = 3
	if len(got) != 3 {
		t.Fatalf("got %d deps, want 3: %v", len(got), got)
	}

	// Sorted: remote first (c < m), then local (my-local), but local deps
	// appear in result. Let's just check the local is there and not indirect.
	var local *Dependency
	for i := range got {
		if got[i].Path == "my-local" {
			local = &got[i]
			break
		}
	}
	if local == nil {
		t.Fatal("local dep not found in results")
	}
	if local.Indirect {
		t.Error("local dep should not be indirect")
	}
	if local.Version != "./libs/local" {
		t.Errorf("local version = %s, want ./libs/local", local.Version)
	}
}

func TestResolveDeps_DirectVersionWins(t *testing.T) {
	src := source.NewMemSource()
	ctx := context.Background()
	cacheDir := "/cache"

	// Direct: A@v1.0.0, D@v2.0.0
	// A's transitive: D@v1.0.0 — direct's v2.0.0 should win.
	src.Files["/cache/codeberg.org/user/a@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/a", "codeberg.org/user/d v1.0.0")

	direct := []Dependency{
		{Path: "codeberg.org/user/a", Version: "v1.0.0"},
		{Path: "codeberg.org/user/d", Version: "v2.0.0"},
	}

	got, err := ResolveDeps(ctx, src, direct, cacheDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var d *Dependency
	for i := range got {
		if got[i].Path == "codeberg.org/user/d" {
			d = &got[i]
			break
		}
	}
	if d == nil {
		t.Fatal("D not found in results")
	}
	if d.Version != "v2.0.0" {
		t.Errorf("D version = %s, want v2.0.0 (direct wins)", d.Version)
	}
	if d.Indirect {
		t.Error("D should be direct (was in direct list)")
	}
}

func TestResolveDeps_IndirectInputStaysIndirect(t *testing.T) {
	src := source.NewMemSource()
	ctx := context.Background()
	cacheDir := "/cache"

	// Simulate a second run: B is already in scampi.mod as indirect from
	// a previous resolution. It should stay indirect in the output.
	src.Files["/cache/codeberg.org/user/a@v1.0.0/scampi.mod"] =
		modContent("codeberg.org/user/a", "codeberg.org/user/b v1.0.0")

	direct := []Dependency{
		{Path: "codeberg.org/user/a", Version: "v1.0.0", Indirect: false},
		{Path: "codeberg.org/user/b", Version: "v1.0.0", Indirect: true},
	}

	got, err := ResolveDeps(ctx, src, direct, cacheDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var b *Dependency
	for i := range got {
		if got[i].Path == "codeberg.org/user/b" {
			b = &got[i]
			break
		}
	}
	if b == nil {
		t.Fatal("B not found in results")
	}
	if !b.Indirect {
		t.Error("B should remain indirect when passed as indirect in input")
	}
}

func assertDep(t *testing.T, got Dependency, path, version string, indirect bool) {
	t.Helper()
	if got.Path != path {
		t.Errorf("path = %s, want %s", got.Path, path)
	}
	if got.Version != version {
		t.Errorf("%s: version = %s, want %s", path, got.Version, version)
	}
	if got.Indirect != indirect {
		t.Errorf("%s: indirect = %v, want %v", path, got.Indirect, indirect)
	}
}
