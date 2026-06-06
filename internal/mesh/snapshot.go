// SPDX-License-Identifier: GPL-3.0-only

package mesh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const snapshotVersion = 1

// snapshot holds last-known-good addresses; alive/dead gets
// re-detected by SWIM after restart.
type snapshot struct {
	Version   int       `json:"version"`
	Self      string    `json:"self"`
	UpdatedAt time.Time `json:"updated_at"`
	Peers     []Peer    `json:"peers"`
}

// readSnapshot returns os.ErrNotExist when the file is absent.
func readSnapshot(path string) (*snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if s.Version != snapshotVersion {
		return nil, fmt.Errorf("%s: unsupported snapshot version %d", path, s.Version)
	}
	return &s, nil
}

// writeSnapshot atomic-renames via a tmp file; creates the
// parent dir if missing.
func writeSnapshot(path string, s *snapshot) error {
	s.Version = snapshotVersion
	s.UpdatedAt = time.Now().UTC()
	s.Peers = dedupePeers(s.Peers)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, "peers-*.json.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func dedupePeers(peers []Peer) []Peer {
	seen := make(map[string]Peer, len(peers))
	for _, p := range peers {
		seen[p.Name] = p
	}
	out := make([]Peer, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
