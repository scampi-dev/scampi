// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/platform"
)

// instanceAddr is the loopback address scampi binds at startup to
// guarantee a single instance per host. Binding succeeds atomically
// for exactly one process; subsequent processes get "address already
// in use" and exit. The port doubles as the future gossip listener
// when the mesh layer lands. 0xFEED (65261) is high in the IANA
// dynamic range so ephemeral collisions are rare.
const instanceAddr = "127.0.0.1:65261"

// helpTemplate renders the full --help output: tagline (Long or
// Short) above the usage block. Clone of cobra's default with the
// tagline routed through our `tagline` func for color.
const helpTemplate = `{{with (or .Long .Short)}}{{tagline (. | trimTrailingWhitespaces)}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

// usageTemplate renders cobra help with our basic-ANSI palette when
// stdout is a tty; falls back to plain text in pipes/redirects.
// Section headers in yellow+bold, command names in cyan, flag names
// in green (descriptions stay plain).
const usageTemplate = `{{header "Usage:"}}{{if .Runnable}}
  {{cmdName .UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{cmdName .CommandPath}} {{cmdName "[command]"}}{{end}}{{if gt (len .Aliases) 0}}

{{header "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{header "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{header "Available Commands:"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{cmdName (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{header "Flags:"}}
{{flagBlock (.LocalFlags.FlagUsages | trimTrailingWhitespaces)}}{{end}}{{if .HasAvailableInheritedFlags}}

{{header "Global Flags:"}}
{{flagBlock (.InheritedFlags.FlagUsages | trimTrailingWhitespaces)}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{cmdName (printf "%s [command] --help" .CommandPath)}}" for more information about a command.{{end}}
`

// cobraColored controls whether the help template funcs emit ANSI
// escapes. main flips it per render target so --help to stdout and
// usage on stderr each follow their own tty.
var cobraColored bool

// colorMode is the parsed value of the --color flag. Package-level so
// decideColor can be called from main() and from the help hook before
// any closure'd state would exist. Defaulted to "auto" so pre-parse
// reads (e.g. very early panics) get sensible behavior.
var colorMode = "auto"

// runtimeReached flips to true once cobra has finished parsing flags
// and validating args. Errors after that point are runtime failures
// (lock contention, file write, ...) and don't earn the usage block.
var runtimeReached bool

// flagLineRe captures the flag-name + type prefix on one line of
// pflag's FlagUsages output. Groups: leading indent, flag prefix
// (possibly with short alias and type), spacing, description.
var flagLineRe = regexp.MustCompile(`^(\s+)((?:-\S, )?--\S+(?: \S+)?)(\s+)(.*)$`)

func registerCobraHelpFuncs() {
	wrap := func(open string) func(string) string {
		return func(s string) string {
			if !cobraColored {
				return s
			}
			return open + s + ansiReset
		}
	}
	cobra.AddTemplateFunc("header", wrap(ansiYellow))
	cobra.AddTemplateFunc("tagline", wrap(ansiBlue))
	cobra.AddTemplateFunc("cmdName", wrap(ansiCyan))
	cobra.AddTemplateFunc("flagBlock", func(s string) string {
		if !cobraColored {
			return s
		}
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			m := flagLineRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			lines[i] = m[1] + ansiGreen + m[2] + ansiReset + m[3] + m[4]
		}
		return strings.Join(lines, "\n")
	})
}

func main() {
	registerCobraHelpFuncs()
	cobraColored = decideColor(colorMode, os.Stdout)
	plat := platform.New()
	ctx, stop := plat.Signals.ShutdownContext(context.Background())
	defer stop()

	root, closeFn := newRootCmd(plat)
	cmd, err := root.ExecuteContextC(ctx)
	_ = closeFn()
	switch {
	case err == nil:
		return
	case errors.Is(err, engine.ErrSnapshotRejected):
		// Engine already emitted snapshot.rejected via the log; we
		// just translate it to an exit code here.
		os.Exit(2)
	case errors.Is(err, engine.ErrApplyFailed):
		os.Exit(1)
	default:
		// Error path writes to stderr; color decision follows stderr.
		errColored := decideColor(colorMode, os.Stderr)
		cobraColored = errColored
		errLine := fmt.Sprintf("Error: %s", err)
		if errColored {
			errLine = ansiRed + errLine + ansiReset
		}
		if runtimeReached {
			// Runtime failure - usage block is noise.
			_, _ = fmt.Fprintln(os.Stderr, errLine)
		} else {
			// CLI-level failure (bad args, unknown flag): show usage
			// for the command the user was actually trying to run.
			cmd.InitDefaultHelpFlag()
			_, _ = fmt.Fprintf(os.Stderr, "%s\n\n%s", errLine, cmd.UsageString())
		}
		os.Exit(1)
	}
}

func newRootCmd(plat platform.Platform) (*cobra.Command, func() error) {
	var (
		actionLogPath string
		actLog        *engine.ActionLog
		inv           *engine.Inventory
		instance      net.Listener
	)
	pickLog := func() engine.Log {
		base := slogEmitter{l: slog.New(&slogHandler{
			out:     os.Stderr,
			colored: decideColor(colorMode, os.Stderr),
			level:   slog.LevelDebug,
		})}
		return engine.NewLog(fanoutEmitter{base, actLog})
	}

	root := &cobra.Command{
		Use:           "scampi",
		Short:         "Decentralized reconciler for bare-metal infrastructure.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			// Validate --color BEFORE marking runtimeReached so a bad
			// value gets the usage block treatment, like other CLI
			// mistakes.
			switch colorMode {
			case "auto", "always", "never":
			default:
				return fmt.Errorf("invalid --color value %q; want auto|always|never", colorMode)
			}
			runtimeReached = true
			// Single-instance enforcement: the bind succeeds for
			// exactly one process per host. Concurrent reconciles
			// can't race because the second scampi never gets past
			// here.
			l, err := net.Listen("tcp", instanceAddr)
			if err != nil {
				return fmt.Errorf("another scampi is already running on this host (could not bind %s)", instanceAddr)
			}
			instance = l
			path := actionLogPath
			if path == "" {
				d, err := plat.Paths.ActionLogDir()
				if err != nil {
					return err
				}
				path = d
			}
			loaded, err := engine.LoadInventory(path)
			if err != nil {
				return fmt.Errorf("action log replay: %w", err)
			}
			inv = loaded
			al, err := engine.NewActionLog(path)
			if err != nil {
				return fmt.Errorf("action log: %w", err)
			}
			actLog = al
			return nil
		},
	}
	root.PersistentFlags().StringVar(&actionLogPath, "action-log", "",
		"action log directory (default: $XDG_STATE_HOME/scampi/actionlog, or /var/lib/scampi/actionlog as root)")
	root.PersistentFlags().StringVar(&colorMode, "color", "auto",
		"colored output: auto|always|never (also honors SCAMPI_COLOR and NO_COLOR env vars)")

	// Help output uses the same color decision as the rest of scampi.
	// SetHelpFunc fires after flag parsing but before the template
	// renders, so colorMode is current by the time we sample it.
	// Children inherit this hook.
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		cobraColored = decideColor(colorMode, os.Stdout)
		defaultHelp(cmd, args)
	})

	apply := &cobra.Command{
		Use:           "apply <dir>",
		Short:         "Reconcile the snapshot in <dir> once.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return engine.Apply(cmd.Context(), args[0], inv, pickLog())
		},
	}

	run := &cobra.Command{
		Use:           "run <dir>",
		Short:         "Watch <dir> and reconcile on every change.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			interval, _ := cmd.Flags().GetDuration("interval")
			return engine.Run(cmd.Context(), args[0], interval, inv, pickLog())
		},
	}
	run.Flags().Duration("interval", 5*time.Second, "poll interval between snapshots")

	root.AddCommand(apply, run)
	for _, c := range []*cobra.Command{root, apply, run} {
		c.SetUsageTemplate(usageTemplate)
		c.SetHelpTemplate(helpTemplate)
	}
	return root, func() error {
		var ferr, lerr error
		if actLog != nil {
			ferr = actLog.Close()
		}
		if instance != nil {
			lerr = instance.Close()
		}
		if ferr != nil {
			return ferr
		}
		return lerr
	}
}
