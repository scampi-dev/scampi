// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/internal/mesh"
	"scampi.dev/scampi/internal/render"
)

func peersCmd() *cli.Command {
	return &cli.Command{
		Name:  "peers",
		Usage: "List peers from the persisted mesh snapshot.",
		Flags: []cli.Flag{
			stateDirFlag(),
			meshBindFlag(),
			instancePortFlag(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			stateDir, err := resolveStateDir(cmd)
			if err != nil {
				return err
			}
			snap, err := mesh.ReadSnapshot(peersFile(stateDir))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			view := render.PeersView{
				Snapshot: snap,
				Stale:    probeStale(instanceAddr(cmd)),
				Now:      time.Now(),
			}
			if cmd.String("output-format") == "json" {
				render.PrintPeersJSON(os.Stdout, view)
				return nil
			}
			render.PrintPeers(os.Stdout, view, decideColor(cmd.String("color"), os.Stdout))
			return nil
		},
	}
}

// probeStale reports whether the instance port is free. A free
// port means no scampi run is on this host, so the snapshot
// reflects state from a no-longer-running process.
func probeStale(addr string) bool {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}
