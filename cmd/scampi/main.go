// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/platform"
	"scampi.dev/scampi/internal/render"
)

var (
	cobraColored   bool
	runtimeReached bool
	plat           platform.Platform
)

//nolint:revive // cobra template; lines are template syntax, not source lines
const helpTemplate = `{{with (or .Long .Short)}}{{tagline (. | trimTrailingWhitespaces)}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

//nolint:revive // cobra template; lines are template syntax, not source lines
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

// First SIGINT goes to the platform's ShutdownContext; a second
// one within the same process lifetime force-exits.
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
	cobraColored = decideColor(os.Stdout)
	plat = platform.New()
	ctx, stop := plat.Signals.ShutdownContext(context.Background())
	defer stop()
	armForceExit()

	root, closeFn := newRootCmd()
	cmd, err := root.ExecuteContextC(ctx)
	_ = closeFn()
	switch {
	case err == nil:
		return
	case errors.Is(err, context.Canceled):
		os.Exit(130)
	case errors.Is(err, engine.ErrSnapshotRejected):
		os.Exit(2)
	case errors.Is(err, engine.ErrReconcileFailed):
		os.Exit(1)
	default:
		errColored := decideColor(os.Stderr)
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
