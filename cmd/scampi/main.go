// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
)

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
	cobraColored = isatty.IsTerminal(os.Stdout.Fd())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, closeFn := newRootCmd()
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
		// Error path writes to stderr, so the color decision for
		// both the Error line and the usage block follows stderr.
		errColored := isatty.IsTerminal(os.Stderr.Fd())
		cobraColored = errColored
		errLine := fmt.Sprintf("Error: %s", err)
		if errColored {
			errLine = ansiRed + errLine + ansiReset
		}
		// Help flag is normally added inside execute(), which the
		// error path bypassed; init it so it shows up in the usage.
		cmd.InitDefaultHelpFlag()
		_, _ = fmt.Fprintf(os.Stderr, "%s\n\n%s", errLine, cmd.UsageString())
		os.Exit(1)
	}
}

// defaultActionLogDir resolves where to write the action log when
// --action-log is not given. Root gets /var/lib/scampi/actionlog;
// everyone else gets $XDG_STATE_HOME/scampi/actionlog with the
// standard XDG fallback to $HOME/.local/state.
func defaultActionLogDir() (string, error) {
	if os.Geteuid() == 0 {
		return "/var/lib/scampi/actionlog", nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "scampi", "actionlog"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "scampi", "actionlog"), nil
}

func newRootCmd() (*cobra.Command, func() error) {
	var (
		actionLogPath string
		colorMode     string
		actLog        *engine.ActionLog
		inv           *engine.Inventory
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
			path := actionLogPath
			if path == "" {
				d, err := defaultActionLogDir()
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
		"colored output: auto|always|never")

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
		if actLog == nil {
			return nil
		}
		return actLog.Close()
	}
}
