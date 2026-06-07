// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"scampi.dev/scampi/internal/mesh"
)

// PeersView is the input to PrintPeers / PrintPeersJSON. Snapshot
// nil means peers.json was absent; Stale true means the probe
// found no live scampi on the instance port.
type PeersView struct {
	Snapshot *mesh.Snapshot
	Stale    bool
	Now      time.Time
}

func PrintPeers(w io.Writer, v PeersView, colored bool) {
	if v.Snapshot == nil {
		_, _ = fmt.Fprintln(w, "no peers known yet")
		return
	}

	dim := func(s string) string {
		if !colored {
			return s
		}
		return AnsiDim + s + AnsiUndim
	}

	_, _ = fmt.Fprintln(w, dim(fmt.Sprintf("snapshot %s (self: %s)",
		humanizeAgo(v.Now, v.Snapshot.UpdatedAt), v.Snapshot.Self)))
	if v.Stale {
		_, _ = fmt.Fprintln(w, dim("(no scampi run active on this host)"))
	}
	_, _ = fmt.Fprintln(w)

	nameWidth, addrWidth := len("NAME"), len("ADDR")
	for _, p := range v.Snapshot.Peers {
		if l := len(p.Name); l > nameWidth {
			nameWidth = l
		}
		if l := len(p.Addr); l > addrWidth {
			addrWidth = l
		}
	}

	header := fmt.Sprintf("%-*s  %-*s  SELF", nameWidth, "NAME", addrWidth, "ADDR")
	if colored {
		header = AnsiBold + header + AnsiUndim
	}
	_, _ = fmt.Fprintln(w, header)
	for _, p := range v.Snapshot.Peers {
		mark := " "
		if p.Name == v.Snapshot.Self {
			mark = "*"
		}
		_, _ = fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameWidth, p.Name, addrWidth, p.Addr, mark)
	}
}

func PrintPeersJSON(w io.Writer, v PeersView) {
	type peerEntry struct {
		Name string `json:"name"`
		Addr string `json:"addr"`
		Self bool   `json:"self"`
	}
	type payload struct {
		Self      string      `json:"self"`
		UpdatedAt time.Time   `json:"updated_at"`
		Stale     bool        `json:"stale"`
		Peers     []peerEntry `json:"peers"`
	}

	out := payload{
		Stale: v.Stale,
		Peers: []peerEntry{},
	}
	if v.Snapshot != nil {
		out.Self = v.Snapshot.Self
		out.UpdatedAt = v.Snapshot.UpdatedAt
		for _, p := range v.Snapshot.Peers {
			out.Peers = append(out.Peers, peerEntry{
				Name: p.Name,
				Addr: p.Addr,
				Self: p.Name == v.Snapshot.Self,
			})
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

// humanizeAgo returns "Xs/Xm/Xh/Xd ago", or "just now" when the
// gap is under a second.
func humanizeAgo(now, then time.Time) string {
	if then.IsZero() {
		return "unknown"
	}
	d := now.Sub(then)
	if d < time.Second {
		return "just now"
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
