// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"scampi.dev/scampi/internal/mesh"
)

func TestPrintPeers_MissingSnapshot(t *testing.T) {
	var buf bytes.Buffer
	PrintPeers(&buf, PeersView{}, false)
	if got := strings.TrimSpace(buf.String()); got != "no peers known yet" {
		t.Errorf("missing snapshot: got %q", got)
	}
}

func TestPrintPeers_FreshSnapshot(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 2, 0, time.UTC)
	snap := &mesh.Snapshot{
		Self:      "peer-a",
		UpdatedAt: now.Add(-2 * time.Second),
		Peers: []mesh.Peer{
			{Name: "peer-a", Addr: "10.0.0.5:65261"},
			{Name: "peer-b", Addr: "10.0.0.6:65261"},
		},
	}
	var buf bytes.Buffer
	PrintPeers(&buf, PeersView{Snapshot: snap, Now: now}, false)
	out := buf.String()
	for _, want := range []string{
		"snapshot 2s ago (self: peer-a)",
		"NAME", "ADDR", "SELF",
		"peer-a", "10.0.0.5:65261", "*",
		"peer-b", "10.0.0.6:65261",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "no scampi run active") {
		t.Errorf("fresh snapshot should not show stale banner; got:\n%s", out)
	}
}

func TestPrintPeers_StaleBanner(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	snap := &mesh.Snapshot{
		Self:      "peer-a",
		UpdatedAt: now.Add(-2 * time.Hour),
		Peers:     []mesh.Peer{{Name: "peer-a", Addr: "10.0.0.5:65261"}},
	}
	var buf bytes.Buffer
	PrintPeers(&buf, PeersView{Snapshot: snap, Stale: true, Now: now}, false)
	out := buf.String()
	if !strings.Contains(out, "snapshot 2h ago") {
		t.Errorf("expected 2h-ago banner; got:\n%s", out)
	}
	if !strings.Contains(out, "(no scampi run active on this host)") {
		t.Errorf("stale banner missing; got:\n%s", out)
	}
}

func TestPrintPeers_ColoredHasAnsi(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	snap := &mesh.Snapshot{
		Self:      "peer-a",
		UpdatedAt: now,
		Peers:     []mesh.Peer{{Name: "peer-a", Addr: "10.0.0.5:65261"}},
	}
	var buf bytes.Buffer
	PrintPeers(&buf, PeersView{Snapshot: snap, Now: now}, true)
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("colored output missing ANSI; got %q", buf.String())
	}
}

func TestPrintPeersJSON_Fresh(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	snap := &mesh.Snapshot{
		Self:      "peer-a",
		UpdatedAt: now,
		Peers: []mesh.Peer{
			{Name: "peer-a", Addr: "10.0.0.5:65261"},
			{Name: "peer-b", Addr: "10.0.0.6:65261"},
		},
	}
	var buf bytes.Buffer
	PrintPeersJSON(&buf, PeersView{Snapshot: snap, Now: now})

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["self"] != "peer-a" {
		t.Errorf("self: got %v want peer-a", got["self"])
	}
	if got["stale"] != false {
		t.Errorf("stale: got %v want false", got["stale"])
	}
	peers, _ := got["peers"].([]any)
	if len(peers) != 2 {
		t.Fatalf("peers: got %d entries; raw:\n%s", len(peers), buf.String())
	}
	p0, _ := peers[0].(map[string]any)
	if p0["name"] != "peer-a" || p0["self"] != true {
		t.Errorf("peer 0 wrong: %v", p0)
	}
	p1, _ := peers[1].(map[string]any)
	if p1["name"] != "peer-b" || p1["self"] != false {
		t.Errorf("peer 1 wrong: %v", p1)
	}
}

func TestPrintPeersJSON_Missing(t *testing.T) {
	var buf bytes.Buffer
	PrintPeersJSON(&buf, PeersView{})
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	peers, _ := got["peers"].([]any)
	if len(peers) != 0 {
		t.Errorf("expected empty peers; got %v", got["peers"])
	}
	if got["self"] != "" {
		t.Errorf("self: got %v want empty", got["self"])
	}
	if got["stale"] != false {
		t.Errorf("stale default: got %v want false", got["stale"])
	}
}

func TestHumanizeAgo(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		gap  time.Duration
		want string
	}{
		{0, "just now"},
		{500 * time.Millisecond, "just now"},
		{2 * time.Second, "2s ago"},
		{90 * time.Second, "1m ago"},
		{2 * time.Hour, "2h ago"},
		{50 * time.Hour, "2d ago"},
	}
	for _, tc := range cases {
		got := humanizeAgo(now, now.Add(-tc.gap))
		if got != tc.want {
			t.Errorf("gap %v: got %q want %q", tc.gap, got, tc.want)
		}
	}
	if got := humanizeAgo(now, time.Time{}); got != "unknown" {
		t.Errorf("zero time: got %q want unknown", got)
	}
}
