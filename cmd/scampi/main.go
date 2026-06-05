// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/platform"
	"scampi.dev/scampi/internal/render"
)

// 0xFEED is high in the IANA dynamic range; ephemeral collisions are rare.
const instanceAddr = "127.0.0.1:65261"

const helpTemplate = `{{with (or .Long .Short)}}{{tagline (. | trimTrailingWhitespaces)}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

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

var (
	cobraColored   bool
	colorMode      = "auto"
	asciiFlag      bool
	verboseCount   int
	quietFlag      bool
	outputFormat   = "text"
	runtimeReached bool
)

func resolveVerbosity() render.Verbosity {
	if quietFlag {
		return render.VerbosityQuiet
	}
	return render.Verbosity(verboseCount)
}

func resolveOutputFormat() string {
	if env := os.Getenv("SCAMPI_OUTPUT_FORMAT"); env != "" {
		return env
	}
	return outputFormat
}

var flagLineRe = regexp.MustCompile(`^(\s+)((?:-\S, )?--\S+(?: \S+)?)(\s+)(.*)$`)

func registerCobraHelpFuncs() {
	wrap := func(open string) func(string) string {
		return func(s string) string {
			if !cobraColored {
				return s
			}
			return open + s + render.AnsiReset
		}
	}
	cobra.AddTemplateFunc("header", wrap(render.AnsiYellow))
	cobra.AddTemplateFunc("tagline", wrap(render.AnsiBlue))
	cobra.AddTemplateFunc("cmdName", wrap(render.AnsiCyan))
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
			lines[i] = m[1] + render.AnsiGreen + m[2] + render.AnsiReset + m[3] + m[4]
		}
		return strings.Join(lines, "\n")
	})
}

// pickApplyEmitter returns the renderer + a finalize callback (json
// mode has no finalize).
func pickApplyEmitter() (engine.Emitter, func(error)) {
	v := resolveVerbosity()
	if resolveOutputFormat() == "json" {
		return render.NewJSONRenderer(os.Stdout, v), func(error) {}
	}
	ar := render.NewApplyRenderer(os.Stdout,
		render.DecideGlyphs(asciiFlag), render.DecideColor(colorMode, os.Stdout), v)
	return ar, ar.Finalize
}

func pickRunEmitter() engine.Emitter {
	v := resolveVerbosity()
	if resolveOutputFormat() == "json" {
		return render.NewJSONRenderer(os.Stdout, v)
	}
	return render.NewRunRenderer(os.Stdout,
		render.DecideGlyphs(asciiFlag), render.DecideColor(colorMode, os.Stdout), v)
}

// armForceExit: first SIGINT goes to the platform's ShutdownContext;
// a second one within the same process lifetime force-exits.
func armForceExit() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		<-ch
		_, _ = fmt.Fprintln(os.Stderr, "force shutdown")
		os.Exit(130)
	}()
}

func main() {
	registerCobraHelpFuncs()
	cobraColored = render.DecideColor(colorMode, os.Stdout)
	plat := platform.New()
	ctx, stop := plat.Signals.ShutdownContext(context.Background())
	defer stop()
	armForceExit()

	root, closeFn := newRootCmd(plat)
	cmd, err := root.ExecuteContextC(ctx)
	_ = closeFn()
	switch {
	case err == nil:
		return
	case errors.Is(err, context.Canceled):
		os.Exit(130)
	case errors.Is(err, engine.ErrSnapshotRejected):
		os.Exit(2)
	case errors.Is(err, engine.ErrApplyFailed):
		os.Exit(1)
	default:
		errColored := render.DecideColor(colorMode, os.Stderr)
		cobraColored = errColored
		errLine := fmt.Sprintf("Error: %s", err)
		if errColored {
			errLine = render.AnsiRed + errLine + render.AnsiReset
		}
		if runtimeReached {
			_, _ = fmt.Fprintln(os.Stderr, errLine)
		} else {
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

	sinkWith := func(r engine.Emitter) engine.Emitter {
		if actLog == nil {
			return r
		}
		return render.FanoutEmitter{r, actLog}
	}

	acquireMutationLock := func() error {
		l, err := net.Listen("tcp", instanceAddr)
		if err != nil {
			return fmt.Errorf("another scampi is already running on this host (could not bind %s)", instanceAddr)
		}
		instance = l
		loaded, err := engine.LoadInventory(actionLogPath)
		if err != nil {
			return fmt.Errorf("action log replay: %w", err)
		}
		inv = loaded
		al, err := engine.NewActionLog(actionLogPath)
		if err != nil {
			return fmt.Errorf("action log: %w", err)
		}
		actLog = al
		return nil
	}

	root := &cobra.Command{
		Use:           "scampi",
		Short:         "Decentralized reconciler for bare-metal infrastructure.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			// Validate before runtimeReached so bad values get the usage block.
			switch colorMode {
			case "auto", "always", "never":
			default:
				return fmt.Errorf("invalid --color value %q; want auto|always|never", colorMode)
			}
			switch resolveOutputFormat() {
			case "text", "json":
			default:
				return fmt.Errorf("invalid --output-format value %q; want text|json",
					resolveOutputFormat())
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
	root.PersistentFlags().StringVar(&actionLogPath, "action-log", "",
		"action log directory (default: $XDG_STATE_HOME/scampi/actionlog, or /var/lib/scampi/actionlog as root)")
	root.PersistentFlags().StringVar(&colorMode, "color", "auto",
		"colored output: auto|always|never (also honors SCAMPI_COLOR and NO_COLOR env vars)")
	root.PersistentFlags().BoolVar(&asciiFlag, "ascii", false,
		"use ASCII glyphs instead of Unicode (also honors SCAMPI_ASCII=1)")
	root.PersistentFlags().CountVarP(&verboseCount, "verbose", "v",
		"increase verbosity (-v shows info, -vv shows debug)")
	root.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false,
		"suppress non-essential output")
	root.PersistentFlags().StringVarP(&outputFormat, "output-format", "o", "text",
		"output format: text|json (also honors SCAMPI_OUTPUT_FORMAT)")

	// SetHelpFunc fires after flag parsing but before the template
	// renders; sample colorMode here so flags take effect.
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		cobraColored = render.DecideColor(colorMode, os.Stdout)
		defaultHelp(cmd, args)
	})

	apply := &cobra.Command{
		Use:           "apply <dir>",
		Short:         "Reconcile the snapshot in <dir> once.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := acquireMutationLock(); err != nil {
				return err
			}
			renderer, finalize := pickApplyEmitter()
			err := engine.Apply(cmd.Context(), engine.ApplyConfig{
				Dir: args[0], Inventory: inv, Log: engine.NewLog(sinkWith(renderer)),
			})
			finalize(err)
			return err
		},
	}

	run := &cobra.Command{
		Use:           "run <dir>",
		Short:         "Watch <dir> and reconcile on every change.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := acquireMutationLock(); err != nil {
				return err
			}
			interval, _ := cmd.Flags().GetDuration("interval")
			renderer := pickRunEmitter()
			return engine.Run(cmd.Context(), engine.RunConfig{
				Dir: args[0], Interval: interval, Inventory: inv, Log: engine.NewLog(sinkWith(renderer)),
			})
		},
	}
	run.Flags().Duration("interval", 5*time.Second, "poll interval between snapshots")

	plan := &cobra.Command{
		Use:           "plan <dir>",
		Short:         "Show what apply would do without changing anything.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := engine.LoadInventoryLenient(actionLogPath)
			if err != nil {
				return fmt.Errorf("action log replay: %w", err)
			}
			inv = loaded
			renderer := pickRunEmitter()
			p, err := engine.MakePlan(cmd.Context(), engine.PlanConfig{
				Dir: args[0], Inventory: inv, Log: engine.NewLog(renderer),
			})
			if err != nil {
				return err
			}
			render.PrintPlan(os.Stdout, p,
				render.DecideGlyphs(asciiFlag), render.DecideColor(colorMode, os.Stdout))
			return nil
		},
	}

	root.AddCommand(apply, run, plan)
	for _, c := range []*cobra.Command{root, apply, run, plan} {
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
