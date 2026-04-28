// SPDX-License-Identifier: GPL-3.0-only

// Package pve provides a target type that proxies posix operations
// into a PVE LXC container via `pct exec` and `pct push`/`pct pull`
// from the PVE host. No SSH connection into the container itself —
// every operation transits through the PVE host's pct CLI.
package pve
