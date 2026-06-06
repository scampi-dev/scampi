// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/mesh"
	"scampi.dev/scampi/internal/render"
)

const (
	meshLeaveTimeout     = 2 * time.Second
	meshSnapshotDebounce = 1 * time.Second
)

func runCmd() *cli.Command {
	var dir string
	return &cli.Command{
		Name:      "run",
		Usage:     "Watch <dir> and reconcile on every change.",
		ArgsUsage: "<dir>",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "dir", Destination: &dir},
		},
		Flags: []cli.Flag{
			&cli.DurationFlag{
				Name:  "interval",
				Value: 5 * time.Second,
				Usage: "poll interval between snapshots",
			},
			&cli.StringFlag{
				Name:    "mesh-advertise",
				Usage:   "address peers reach this node on; empty auto-detects",
				Sources: cli.EnvVars("SCAMPI_MESH_ADVERTISE"),
			},
			&cli.StringFlag{
				Name:    "mesh-name",
				Usage:   "node identity in the mesh; empty defaults to hostname",
				Sources: cli.EnvVars("SCAMPI_MESH_NAME"),
			},
			&cli.StringFlag{
				Name:    "join",
				Usage:   "comma-separated seed host:port for first-ever join",
				Sources: cli.EnvVars("SCAMPI_MESH_JOIN"),
			},
		},
		Before: requireArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			actionLogDir, err := resolveActionLogDir(cmd)
			if err != nil {
				return err
			}
			snapPath, err := plat.Paths.PeersFile()
			if err != nil {
				return err
			}
			emitter := pickRunEmitter(cmd)

			m, merr := startMesh(ctx, cmd, engine.NewLog(emitter), snapPath)
			if merr != nil {
				emitter.Emit(ctx, engine.CodeMeshUnavailable, nil, "err", merr.Error())
			} else {
				emitter.Emit(
					ctx, engine.CodeMeshUp, nil,
					"name", m.Self().Name,
					"addr", m.Self().Addr,
					"members", len(m.Members()),
				)
				go forwardMeshEvents(ctx, emitter, m)
				defer func() {
					_ = m.Leave(meshLeaveTimeout)
					_ = m.Shutdown()
					emitter.Emit(ctx, engine.CodeMeshDown, nil)
				}()
			}

			return engine.Run(ctx, engine.RunConfig{
				Dir:          dir,
				ActionLogDir: actionLogDir,
				Emitter:      emitter,
				Interval:     cmd.Duration("interval"),
			})
		},
	}
}

func pickRunEmitter(cmd *cli.Command) engine.Emitter {
	v := resolveVerbosity(cmd)
	if cmd.String("output-format") == "json" {
		return render.NewJSONRenderer(os.Stdout, v)
	}
	return render.NewRunRenderer(
		os.Stdout,
		decideGlyphs(cmd.Bool("ascii")),
		decideColor(cmd.String("color"), os.Stdout),
		v,
	)
}

func startMesh(
	ctx context.Context,
	cmd *cli.Command,
	log engine.Log,
	snapPath string,
) (*mesh.Mesh, error) {
	port := int(cmd.Int("instance-port"))
	return mesh.Run(ctx, mesh.Config{
		Name:             defaultMeshName(cmd.String("mesh-name"), port),
		BindAddr:         cmd.String("mesh-bind"),
		BindPort:         port,
		AdvertiseAddr:    cmd.String("mesh-advertise"),
		AdvertisePort:    port,
		Join:             parseJoinSeeds(cmd.String("join")),
		SnapshotPath:     snapPath,
		Logger:           log,
		SnapshotDebounce: meshSnapshotDebounce,
	})
}

func defaultMeshName(meshName string, port int) string {
	if meshName != "" {
		return meshName
	}
	h, _ := os.Hostname()
	if port != defaultInstancePort {
		return fmt.Sprintf("%s-%d", h, port)
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
