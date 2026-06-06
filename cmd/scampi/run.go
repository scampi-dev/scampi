// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/render"
)

var (
	meshAdvertise string
	meshName      string
	joinSeeds     string
)

func pickRunEmitter() engine.Emitter {
	v := resolveVerbosity()
	if outputFormat == "json" {
		return render.NewJSONRenderer(os.Stdout, v)
	}
	return render.NewRunRenderer(os.Stdout, decideGlyphs(), decideColor(os.Stdout), v)
}

// defaultMeshName uses the configured --mesh-name; otherwise the
// hostname. Suffixes the port for non-default ports so two peers
// on one host don't collide.
func defaultMeshName() string {
	if meshName != "" {
		return meshName
	}
	h, _ := os.Hostname()
	if instancePort != defaultInstancePort {
		return fmt.Sprintf("%s-%d", h, instancePort)
	}
	return h
}

func parseJoinSeeds(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newRunCmd() *cobra.Command {
	var interval time.Duration

	cmd := &cobra.Command{
		Use:           "run <dir>",
		Short:         "Watch <dir> and reconcile on every change.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			snapPath, err := plat.Paths.PeersFile()
			if err != nil {
				return err
			}
			return engine.Run(cmd.Context(), engine.RunConfig{
				Dir:          args[0],
				ActionLogDir: actionLogPath,
				Emitter:      pickRunEmitter(),
				Interval:     interval,
				Mesh: &engine.MeshConfig{
					Name:          defaultMeshName(),
					BindAddr:      meshBind,
					BindPort:      instancePort,
					AdvertiseAddr: meshAdvertise,
					AdvertisePort: instancePort,
					Join:          parseJoinSeeds(joinSeeds),
					SnapshotPath:  snapPath,
				},
			})
		},
	}
	cmd.Flags().DurationVar(
		&interval, "interval", 5*time.Second,
		"poll interval between snapshots",
	)
	cmd.Flags().StringVar(
		&meshAdvertise, "mesh-advertise",
		envOr("SCAMPI_MESH_ADVERTISE", ""),
		"address peers reach this node on; empty auto-detects",
	)
	cmd.Flags().StringVar(
		&meshName, "mesh-name",
		envOr("SCAMPI_MESH_NAME", ""),
		"node identity in the mesh; empty defaults to hostname",
	)
	cmd.Flags().StringVar(
		&joinSeeds, "join",
		envOr("SCAMPI_MESH_JOIN", ""),
		"comma-separated seed host:port for first-ever join",
	)
	return cmd
}
