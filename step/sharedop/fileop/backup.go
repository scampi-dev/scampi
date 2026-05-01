// SPDX-License-Identifier: GPL-3.0-only

package fileop

import (
	"context"
	"time"

	"scampi.dev/scampi/target"
)

// Backup copies the existing file at dest to a timestamped backup
// (dest.20260422T120000.bak) before it is overwritten. Never
// overwrites an existing backup. If the file does not exist, this
// is a no-op.
func Backup(ctx context.Context, tgt target.Filesystem, dest string) error {
	data, err := tgt.ReadFile(ctx, dest)
	if err != nil {
		return nil // file doesn't exist — nothing to back up
	}
	stamp := time.Now().UTC().Format("20060102T150405")
	return tgt.WriteFile(ctx, dest+"."+stamp+".bak", data)
}
