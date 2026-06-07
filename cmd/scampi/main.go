// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/platform"
	"scampi.dev/scampi/internal/render"
)

var (
	plat        platform.Platform
	helpColored bool
)

func main() {
	helpColored = colorEnabledForHelp()
	if helpColored {
		colorizeHelpTemplates()
	}
	cli.FlagStringer = cobraFlagStringer

	plat = platform.New()
	ctx, stop := plat.Signals.ShutdownContext(context.Background())
	defer stop()

	app := &cli.Command{
		Name:                   "scampi",
		Usage:                  "Decentralized reconciler for bare-metal infrastructure.",
		Suggest:                true,
		UseShortOptionHandling: true,
		CommandNotFound:        commandNotFound,
		Flags:                  rootFlags(),
		Commands: []*cli.Command{
			reconcileCmd(),
			runCmd(),
			planCmd(),
			peersCmd(),
		},
	}

	err := app.Run(ctx, expandShorthand(os.Args, app.Commands))
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
		errLine := fmt.Sprintf("Error: %s", err)
		if isatty.IsTerminal(os.Stderr.Fd()) {
			errLine = render.AnsiRed + errLine + render.AnsiReset
		}
		_, _ = fmt.Fprintln(os.Stderr, errLine)
		os.Exit(1)
	}
}

func commandNotFound(_ context.Context, cmd *cli.Command, name string) {
	errLine := fmt.Sprintf("Error: unknown command %q for %q", name, cmd.Name)
	if errColored() {
		errLine = render.AnsiRed + errLine + render.AnsiReset
	}
	_, _ = fmt.Fprintln(os.Stderr, errLine)
	if s := cli.SuggestCommand(cmd.Commands, name); s != "" {
		_, _ = fmt.Fprintf(os.Stderr, "\nDid you mean %q?\n", s)
	}
	_, _ = fmt.Fprintln(os.Stderr)
	_ = cli.ShowAppHelp(cmd)
	cli.OsExiter(1)
}

func colorEnabledForHelp() bool { return resolveEarlyColor(os.Stdout) }
func errColored() bool          { return resolveEarlyColor(os.Stderr) }

// resolveEarlyColor decides color before cli parses flags.
// Pre-scans os.Args so `--color=always` works under wrappers like
// `watch` that pipe stdout (defeats tty-detect).
func resolveEarlyColor(w *os.File) bool {
	if v := argColor(); v != "" {
		return v == "always"
	}
	switch os.Getenv("SCAMPI_COLOR") {
	case "always":
		return true
	case "never":
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isatty.IsTerminal(w.Fd())
}

func argColor() string {
	for i, a := range os.Args {
		switch {
		case strings.HasPrefix(a, "--color="):
			return strings.TrimPrefix(a, "--color=")
		case a == "--color" && i+1 < len(os.Args):
			return os.Args[i+1]
		}
	}
	return ""
}

// colorizeHelpTemplates wraps section headers in the three urfave
// help templates with ANSI yellow.
func colorizeHelpTemplates() {
	// Longest first so "GLOBAL OPTIONS:" matches before the
	// "OPTIONS:" substring rule eats its tail.
	headers := []string{
		"GLOBAL OPTIONS:", "DESCRIPTION:", "COPYRIGHT:", "CATEGORY:",
		"COMMANDS:", "VERSION:", "OPTIONS:", "AUTHOR:",
		"USAGE:", "NAME:",
	}
	colorize := func(t string) string {
		// Sentinels so the second pass can't re-wrap an "OPTIONS:"
		// that lives inside an already-colored "GLOBAL OPTIONS:".
		for i, h := range headers {
			t = strings.ReplaceAll(t, h, fmt.Sprintf("\x00H%d\x00", i))
		}
		for i, h := range headers {
			t = strings.ReplaceAll(t, fmt.Sprintf("\x00H%d\x00", i),
				render.AnsiYellow+h+render.AnsiReset)
		}
		return t
	}
	cli.RootCommandHelpTemplate = colorize(cli.RootCommandHelpTemplate)
	cli.CommandHelpTemplate = colorize(cli.CommandHelpTemplate)
	cli.SubcommandHelpTemplate = colorize(cli.SubcommandHelpTemplate)
}

// cobraFlagStringer renders flags cobra-style: short alias first,
// long after, single type annotation. Long-only flags get padded
// so they line up under the short-flagged ones.
func cobraFlagStringer(f cli.Flag) string {
	df, ok := f.(cli.DocGenerationFlag)
	if !ok {
		return ""
	}
	var short, long []string
	for _, n := range f.Names() {
		if len(n) == 1 {
			short = append(short, "-"+n)
		} else {
			long = append(long, "--"+n)
		}
	}
	var head strings.Builder
	switch {
	case len(short) > 0 && len(long) > 0:
		_, _ = head.WriteString(strings.Join(short, ", "))
		_, _ = head.WriteString(", ")
		_, _ = head.WriteString(strings.Join(long, ", "))
	case len(short) > 0:
		_, _ = head.WriteString(strings.Join(short, ", "))
	default:
		// 4 spaces line long-only flags up with "-X, --..." form.
		_, _ = head.WriteString("    ")
		_, _ = head.WriteString(strings.Join(long, ", "))
	}
	if df.TakesValue() {
		if t := df.TypeName(); t != "" {
			_, _ = head.WriteString(" ")
			_, _ = head.WriteString(t)
		}
	}

	usage := df.GetUsage()
	if df.IsDefaultVisible() {
		if s := df.GetDefaultText(); s != "" {
			usage += " (default: " + s + ")"
		} else if df.TakesValue() && df.GetValue() != "" {
			usage += " (default: " + df.GetValue() + ")"
		}
	}
	if env := df.GetEnvVars(); len(env) > 0 {
		usage += " [$" + strings.Join(env, ", $") + "]"
	}
	headStr := head.String()
	if helpColored {
		headStr = render.AnsiGreen + headStr + render.AnsiReset
	}
	return fmt.Sprintf("%s\t%s", headStr, strings.TrimSpace(usage))
}
