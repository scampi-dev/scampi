// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/mesh"
	"scampi.dev/scampi/internal/render"
)

const (
	meshLeaveTimeout     = 2 * time.Second
	meshSnapshotDebounce = 1 * time.Second
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

func startMesh(ctx context.Context, log engine.Log, snapPath string) (*mesh.Mesh, error) {
	return mesh.Run(ctx, mesh.Config{
		Name:             defaultMeshName(),
		BindAddr:         meshBind,
		BindPort:         instancePort,
		AdvertiseAddr:    meshAdvertise,
		AdvertisePort:    instancePort,
		Join:             parseJoinSeeds(joinSeeds),
		SnapshotPath:     snapPath,
		Logger:           log,
		SnapshotDebounce: meshSnapshotDebounce,
	})
}

func forwardMeshEvents(ctx context.Context, emitter engine.Emitter, m *mesh.Mesh) {
	for ev := range m.Events() {
		var code engine.Code
		switch ev.Kind {
		case mesh.EventJoin:
			code = engine.CodeMeshPeerJoined
		case mesh.EventLeave:
			code = engine.CodeMeshPeerLeft
		case mesh.EventUpdate:
			code = engine.CodeMeshPeerUpdated
		default:
			continue
		}
		emitter.Emit(ctx, code, nil, "name", ev.Peer.Name, "addr", ev.Peer.Addr)
	}
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
			emitter := pickRunEmitter()

			m, merr := startMesh(cmd.Context(), engine.NewLog(emitter), snapPath)
			if merr != nil {
				emitter.Emit(
					cmd.Context(), engine.CodeMeshUnavailable, nil,
					"err", merr.Error(),
				)
			} else {
				emitter.Emit(
					cmd.Context(), engine.CodeMeshUp, nil,
					"name", m.Self().Name,
					"addr", m.Self().Addr,
					"members", len(m.Members()),
				)
				go forwardMeshEvents(cmd.Context(), emitter, m)
				defer func() {
					_ = m.Leave(meshLeaveTimeout)
					_ = m.Shutdown()
					emitter.Emit(cmd.Context(), engine.CodeMeshDown, nil)
				}()
			}

			return engine.Run(cmd.Context(), engine.RunConfig{
				Dir:          args[0],
				ActionLogDir: actionLogPath,
				Emitter:      emitter,
				Interval:     interval,
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
