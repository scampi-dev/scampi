// SPDX-License-Identifier: GPL-3.0-only

package mesh

import (
	"net"
	"path/filepath"
	"testing"
	"time"
)

const testWait = 3 * time.Second

func TestMesh_ColdStart(t *testing.T) {
	m := startMesh(t, "peer-a", nil, "")
	if !m.Healthy() {
		t.Error("expected Healthy on fresh start")
	}
	if got := m.Self().Name; got != "peer-a" {
		t.Errorf("self name: got %s want peer-a", got)
	}
	if got := len(m.Members()); got != 1 {
		t.Errorf("members on cold start: got %d want 1 (self only)", got)
	}
}

func TestMesh_TwoNodeJoin(t *testing.T) {
	a := startMesh(t, "peer-a", nil, "")
	b := startMesh(t, "peer-b", []string{a.Self().Addr}, "")
	waitForMembers(t, a, 2)
	waitForMembers(t, b, 2)
}

func TestMesh_ThreeNodeJoin(t *testing.T) {
	a := startMesh(t, "peer-a", nil, "")
	b := startMesh(t, "peer-b", []string{a.Self().Addr}, "")
	c := startMesh(t, "peer-c", []string{a.Self().Addr}, "")
	waitForMembers(t, a, 3)
	waitForMembers(t, b, 3)
	waitForMembers(t, c, 3)
}

func TestMesh_GracefulLeaveEmitsEvent(t *testing.T) {
	a := startMesh(t, "peer-a", nil, "")
	b := startMesh(t, "peer-b", []string{a.Self().Addr}, "")
	waitForEvent(t, b, EventJoin, "peer-a")

	if err := a.Leave(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	waitForEvent(t, b, EventLeave, "peer-a")
}

func TestMesh_SnapshotWrittenWithPeers(t *testing.T) {
	snapPath := filepath.Join(t.TempDir(), "peers.json")
	a := startMesh(t, "peer-a", nil, "")
	startMesh(t, "peer-b", []string{a.Self().Addr}, snapPath)

	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		if snap, err := readSnapshot(snapPath); err == nil && len(snap.Peers) >= 2 {
			if snap.Self != "peer-b" {
				t.Errorf("snapshot self: got %s want peer-b", snap.Self)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("snapshot never reached 2 peers")
}

func TestMesh_PersistedRejoin(t *testing.T) {
	snapPath := filepath.Join(t.TempDir(), "peers.json")

	a := startMesh(t, "peer-a", nil, "")
	b1 := startMesh(t, "peer-b", []string{a.Self().Addr}, snapPath)
	waitForMembers(t, b1, 2)
	if err := b1.Shutdown(); err != nil {
		t.Fatal(err)
	}
	waitForMembers(t, a, 1)

	snap, err := readSnapshot(snapPath)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	foundA := false
	for _, p := range snap.Peers {
		if p.Name == "peer-a" {
			foundA = true
		}
	}
	if !foundA {
		t.Fatalf("snapshot missing peer-a: %+v", snap.Peers)
	}
	// no --join; should rejoin via snapshot
	b2 := startMesh(t, "peer-b", nil, snapPath)
	waitForMembers(t, b2, 2)
	waitForMembers(t, a, 2)
}

// helpers
// -----------------------------------------------------------------------------

func startMesh(t *testing.T, name string, join []string, snapPath string) *Mesh {
	t.Helper()
	port := pickPort(t)
	cfg := Config{
		Name:                name,
		BindAddr:            "127.0.0.1",
		BindPort:            port,
		AdvertiseAddr:       "127.0.0.1",
		AdvertisePort:       port,
		Join:                join,
		SnapshotPath:        snapPath,
		ProbeInterval:       100 * time.Millisecond,
		SuspectMult:         2,
		DeadNodeReclaimTime: 1 * time.Millisecond,
		SnapshotDebounce:    0,
	}
	m, err := Run(t.Context(), cfg)
	if err != nil {
		t.Fatalf("Run(%s): %v", name, err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	return m
}

func pickPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func waitForMembers(t *testing.T, m *Mesh, n int) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		if len(m.Members()) == n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("waited %v, got %d members, want %d", testWait, len(m.Members()), n)
}

func waitForEvent(t *testing.T, m *Mesh, kind EventKind, peerName string) {
	t.Helper()
	deadline := time.After(testWait)
	for {
		select {
		case ev, ok := <-m.Events():
			if !ok {
				t.Fatal("events channel closed before expected event")
			}
			if ev.Kind == kind && ev.Peer.Name == peerName {
				return
			}
		case <-deadline:
			t.Fatalf("timeout: expected %v for %s", kind, peerName)
		}
	}
}
