// SPDX-License-Identifier: GPL-3.0-only

package mesh

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshot_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	in := &snapshot{
		Self: "peer-a",
		Peers: []Peer{
			{Name: "peer-a", Addr: "10.0.0.5:7946"},
			{Name: "peer-b", Addr: "10.0.0.6:7946"},
		},
	}
	if err := writeSnapshot(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := readSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Self != "peer-a" {
		t.Errorf("self: got %q want %q", out.Self, "peer-a")
	}
	if out.Version != snapshotVersion {
		t.Errorf("version: got %d want %d", out.Version, snapshotVersion)
	}
	if out.UpdatedAt.IsZero() {
		t.Error("updated_at: expected non-zero stamp")
	}
	if len(out.Peers) != 2 || out.Peers[0].Name != "peer-a" || out.Peers[1].Name != "peer-b" {
		t.Errorf("peers: got %+v", out.Peers)
	}
}

func TestSnapshot_ReadMissing(t *testing.T) {
	_, err := readSnapshot(filepath.Join(t.TempDir(), "absent.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("got %v, want errors.Is(_, os.ErrNotExist)", err)
	}
}

func TestSnapshot_ReadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSnapshot(path); err == nil {
		t.Error("expected decode error, got nil")
	}
}

func TestSnapshot_VersionMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	data, err := json.Marshal(map[string]any{
		"version": 999,
		"self":    "peer-a",
		"peers":   []Peer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSnapshot(path); err == nil {
		t.Error("expected version mismatch error, got nil")
	}
}

func TestSnapshot_DedupesAndSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	in := &snapshot{
		Self: "peer-a",
		Peers: []Peer{
			{Name: "peer-c", Addr: "10.0.0.7:7946"},
			{Name: "peer-a", Addr: "10.0.0.5:7946"},
			{Name: "peer-a", Addr: "10.0.0.5:7946"},
			{Name: "peer-b", Addr: "10.0.0.6:7946"},
		},
	}
	if err := writeSnapshot(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := readSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Peers) != 3 {
		t.Fatalf("dedupe: got %d peers want 3 (%+v)", len(out.Peers), out.Peers)
	}
	want := []string{"peer-a", "peer-b", "peer-c"}
	for i, w := range want {
		if out.Peers[i].Name != w {
			t.Errorf("peers[%d]: got %s want %s", i, out.Peers[i].Name, w)
		}
	}
}

func TestSnapshot_AtomicRenameNoTmpLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")
	s := &snapshot{Self: "peer-a", Peers: []Peer{{Name: "peer-a", Addr: "127.0.0.1:7946"}}}
	if err := writeSnapshot(path, s); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "peers.json" {
			t.Errorf("leftover file: %s", e.Name())
		}
	}
}

func TestSnapshot_CreatesParentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "scampi")
	path := filepath.Join(dir, "peers.json")
	s := &snapshot{Self: "peer-a", Peers: []Peer{{Name: "peer-a", Addr: "127.0.0.1:7946"}}}
	if err := writeSnapshot(path, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("written file not present: %v", err)
	}
}
