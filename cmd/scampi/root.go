// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/internal/render"
)

const defaultInstancePort = 0xfeed

func rootFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:    "ascii",
			Usage:   "use ASCII glyphs instead of Unicode",
			Sources: cli.EnvVars("SCAMPI_ASCII"),
		},
		&cli.StringFlag{
			Name:    "color",
			Value:   "auto",
			Usage:   "colored output: auto|always|never (NO_COLOR also honored)",
			Sources: cli.EnvVars("SCAMPI_COLOR"),
		},
		&cli.StringFlag{
			Name:    "output-format",
			Aliases: []string{"o"},
			Value:   "text",
			Usage:   "output format: text|json",
			Sources: cli.EnvVars("SCAMPI_OUTPUT_FORMAT"),
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "suppress non-essential output",
			Sources: cli.EnvVars("SCAMPI_QUIET"),
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "increase verbosity (-v shows info, -vv shows debug)",
		},
	}
}

func stateDirFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "state-dir",
		Usage:   "scampi state dir (action log + mesh peers; defaults under XDG state dir)",
		Sources: cli.EnvVars("SCAMPI_STATE_DIR"),
	}
}

func meshBindFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "mesh-bind",
		Value:   "0.0.0.0",
		Usage:   "bind interface for the mesh port",
		Sources: cli.EnvVars("SCAMPI_MESH_BIND"),
	}
}

func instancePortFlag() cli.Flag {
	return &cli.IntFlag{
		Name:    "instance-port",
		Value:   defaultInstancePort,
		Usage:   "single-instance lock and mesh SWIM port",
		Sources: cli.EnvVars("SCAMPI_INSTANCE_PORT"),
	}
}

func requireArgs(n int) func(context.Context, *cli.Command) (context.Context, error) {
	return func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Args().Len() != n {
			cli.ShowCommandHelpAndExit(ctx, cmd.Root(), cmd.Name, 1)
		}
		return ctx, nil
	}
}

func resolveStateDir(cmd *cli.Command) (string, error) {
	if d := cmd.String("state-dir"); d != "" {
		return d, nil
	}
	return plat.Paths.StateDir()
}

func actionLogDir(stateDir string) string { return filepath.Join(stateDir, "log") }
func peersFile(stateDir string) string    { return filepath.Join(stateDir, "peers.json") }

func resolveVerbosity(cmd *cli.Command) render.Verbosity {
	if cmd.Bool("quiet") {
		return render.VerbosityQuiet
	}
	return render.Verbosity(cmd.Count("verbose"))
}

// decideColor priority: always > NO_COLOR > never > tty-detect.
// mode comes from --color (which picks up SCAMPI_COLOR via Sources).
func decideColor(mode string, w *os.File) bool {
	if mode == "always" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if mode == "never" {
		return false
	}
	return isatty.IsTerminal(w.Fd())
}

func decideGlyphs(asciiFlag bool) render.Glyphs {
	if asciiFlag {
		return render.ASCIIGlyphs
	}
	return render.UnicodeGlyphs
}

func instanceAddr(cmd *cli.Command) string {
	return net.JoinHostPort(
		cmd.String("mesh-bind"),
		strconv.Itoa(int(cmd.Int("instance-port"))),
	)
}

func acquireInstanceListener(addr string) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf(
			"another scampi is already running on this host (could not bind %s)",
			addr,
		)
	}
	return l, nil
}

// expandShorthand finds the first non-flag arg and, if it uniquely
// prefixes one subcommand name, rewrites it to the full name.
// kubectl-style: `scampi recon` -> `scampi reconcile`.
func expandShorthand(args []string, commands []*cli.Command) []string {
	for i, a := range args {
		if i == 0 || strings.HasPrefix(a, "-") {
			continue
		}
		var matches []string
		for _, c := range commands {
			if c.Name == a {
				return args // exact match; nothing to do
			}
			if strings.HasPrefix(c.Name, a) {
				matches = append(matches, c.Name)
			}
		}
		if len(matches) == 1 {
			out := make([]string, len(args))
			copy(out, args)
			out[i] = matches[0]
			return out
		}
		return args // unknown or ambiguous; let urfave handle it
	}
	return args
}
