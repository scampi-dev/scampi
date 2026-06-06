// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/render"
)

// 0xfeed is high in the IANA dynamic range; ephemeral collisions
// are rare. Overridable for multi-instance dev and mesh testing.
const defaultInstancePort = 0xfeed

var (
	actionLogPath string
	instance      net.Listener

	asciiFlag    bool
	verboseCount int
	quietFlag    bool
	instancePort int
	meshBind     string

	colorMode    = "auto"
	outputFormat = "text"
)

func envOr(envKey, fallback string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return fallback
}

func envIntOr(envKey string, fallback int) int {
	if v := os.Getenv(envKey); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envBool(envKey string, fallback bool) bool {
	if v := os.Getenv(envKey); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func resolveVerbosity() render.Verbosity {
	if quietFlag {
		return render.VerbosityQuiet
	}
	return render.Verbosity(verboseCount)
}

// decideColor priority: always > NO_COLOR > never > tty-detect.
// colorMode picks up SCAMPI_COLOR via envOr at flag registration.
func decideColor(w *os.File) bool {
	if colorMode == "always" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if colorMode == "never" {
		return false
	}
	return isatty.IsTerminal(w.Fd())
}

// decideGlyphs picks ASCII over Unicode based on the ascii flag.
// No tty-detect; non-UTF8 environments opt in explicitly.
func decideGlyphs() render.Glyphs {
	if asciiFlag {
		return render.ASCIIGlyphs
	}
	return render.UnicodeGlyphs
}

func instanceAddr() string {
	return net.JoinHostPort(meshBind, strconv.Itoa(instancePort))
}

func acquireInstanceListener() (net.Listener, error) {
	addr := instanceAddr()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf(
			"another scampi is already running on this host (could not bind %s)",
			addr,
		)
	}
	return l, nil
}

func newRootCmd() (*cobra.Command, func() error) {
	root := &cobra.Command{
		Use:           "scampi",
		Short:         "Decentralized reconciler for bare-metal infrastructure.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			switch colorMode {
			case "auto", "always", "never":
			default:
				return fmt.Errorf(
					"invalid --color value %q; want auto|always|never",
					colorMode,
				)
			}
			switch outputFormat {
			case "text", "json":
			default:
				return fmt.Errorf(
					"invalid --output-format value %q; want text|json",
					outputFormat,
				)
			}
			runtimeReached = true
			if actionLogPath == "" {
				d, err := plat.Paths.ActionLogDir()
				if err != nil {
					return err
				}
				actionLogPath = d
			}
			return nil
		},
	}
	root.PersistentFlags().StringVar(
		&actionLogPath,
		"action-log",
		"",
		"action log dir ($XDG_STATE_HOME/scampi/actionlog; /var/lib/scampi/actionlog as root)",
	)
	root.PersistentFlags().StringVar(
		&colorMode,
		"color",
		envOr("SCAMPI_COLOR", "auto"),
		"colored output: auto|always|never (also honors NO_COLOR)",
	)
	root.PersistentFlags().BoolVar(
		&asciiFlag,
		"ascii",
		envBool("SCAMPI_ASCII", false),
		"use ASCII glyphs instead of Unicode",
	)
	root.PersistentFlags().CountVarP(
		&verboseCount,
		"verbose",
		"v",
		"increase verbosity (-v shows info, -vv shows debug)",
	)
	root.PersistentFlags().BoolVarP(
		&quietFlag,
		"quiet",
		"q",
		envBool("SCAMPI_QUIET", false),
		"suppress non-essential output",
	)
	root.PersistentFlags().StringVarP(
		&outputFormat,
		"output-format",
		"o",
		envOr("SCAMPI_OUTPUT_FORMAT", "text"),
		"output format: text|json",
	)
	root.PersistentFlags().IntVar(
		&instancePort,
		"instance-port",
		envIntOr("SCAMPI_INSTANCE_PORT", defaultInstancePort),
		"single-instance lock and mesh SWIM port",
	)
	root.PersistentFlags().StringVar(
		&meshBind,
		"mesh-bind",
		envOr("SCAMPI_MESH_BIND", "0.0.0.0"),
		"bind interface for the mesh port",
	)

	// SetHelpFunc fires after flag parsing but before the template
	// renders; sample colorMode here so flags take effect.
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		cobraColored = decideColor(os.Stdout)
		defaultHelp(cmd, args)
	})

	apply := newApplyCmd()
	run := newRunCmd()
	plan := newPlanCmd()
	root.AddCommand(apply, run, plan)
	for _, c := range []*cobra.Command{root, apply, run, plan} {
		c.SetUsageTemplate(usageTemplate)
		c.SetHelpTemplate(helpTemplate)
	}

	return root, func() error {
		if instance != nil {
			return instance.Close()
		}
		return nil
	}
}
