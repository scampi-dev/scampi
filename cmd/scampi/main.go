// SPDX-License-Identifier: GPL-3.0-only

// Package main defines the CLI surface of scampi.
//
// It wires user-facing commands to engine execution, diagnostics, and rendering.
// This package contains no execution semantics; it is responsible only for
// argument parsing, command dispatch, and process exit behavior.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	ossig "os/signal"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/urfave/cli/v3"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/osutil"
	clir "scampi.dev/scampi/render/cli"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

// interruptHook holds a function called on SIGINT to notify the active
// displayer. Protected by interruptMu because the signal goroutine and
// command actions run concurrently.
var (
	interruptMu   sync.Mutex
	interruptHook func()
)

var version = "v0.0.0-dev"

const (
	exitOK        = 0 // success
	exitUserError = 1 // invalid config, failed plan, validation errors
	exitBug       = 2 // internal error / panic
)

type (
	ctxKey     string
	globalOpts struct {
		ascii     bool
		colorMode signal.ColorMode
		verbosity signal.Verbosity
	}
)

const (
	flagASCII     = "ascii"
	flagColor     = "color"
	flagVerbosity = "v"

	// Resolve options flags
	flagOnly    = "only"
	flagTargets = "targets"

	ctxGlobalOpts = ctxKey("globalOpts")
)

func main() {
	scampi := &cli.Command{
		Name:                   "scampi",
		Usage:                  "Declarative task execution for local and remote systems",
		Version:                version,
		HideVersion:            true,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  flagASCII,
				Usage: "force ASCII output (disable fancy glyphs)",
			},
			&cli.StringFlag{
				Name:  flagColor,
				Value: "auto",
				Usage: "colorize output: auto, always, never",
				Validator: func(s string) error {
					switch strings.ToLower(s) {
					case "auto", "always", "never":
						return nil
					default:
						// bare-error: CLI flag validation, not reachable through engine
						return errs.Errorf("invalid --color value %q (expected auto, always, or never)", s)
					}
				},
			},
			&cli.BoolFlag{
				Name:  flagVerbosity,
				Usage: "increase verbosity (-v, -vv, -vvv)",
			},
		},
		Commands: []*cli.Command{
			fmtCmd(),
			planCmd(),
			checkCmd(),
			applyCmd(),
			inspectCmd(),
			genCmd(),
			modCmd(),
			testCmd(),
			indexCmd(),
			secretsCmd(),
			legendCmd(),
			versionCmd(),
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			ascii := cmd.Bool(flagASCII)
			verbosity := parseVerbosity(cmd)
			colorMode, err := parseColorMode(cmd)
			if err != nil {
				return nil, cli.Exit(err.Error(), exitUserError)
			}

			opts := globalOpts{
				ascii:     ascii,
				colorMode: colorMode,
				verbosity: verbosity,
			}

			return context.WithValue(ctx, ctxGlobalOpts, opts), nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	ossig.Notify(sigCh, osutil.MainContextSignals...)
	go func() {
		<-sigCh
		interruptMu.Lock()
		if interruptHook != nil {
			interruptHook()
		}
		interruptMu.Unlock()
		cancel()
		ossig.Stop(sigCh)
	}()

	if err := scampi.Run(ctx, os.Args); err != nil {
		// Flag-parsing errors (missing value, unknown flag) are reported
		// to stderr by urfave/cli before returning. Exit cleanly so the
		// user sees only the library's message, not a scary "unhandled
		// error" trace.
		var exitErr cli.ExitCoder
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(exitUserError)
	}
}

// Argument validation
// -----------------------------------------------------------------------------

func onUsageError(_ context.Context, cmd *cli.Command, err error, _ bool) error {
	_, _ = fmt.Fprintf(os.Stderr, "Incorrect Usage: %s\n\n", err)
	_ = cli.ShowSubcommandHelp(cmd)
	return cli.Exit("", exitUserError)
}

func requireMinArgs(n int) func(context.Context, *cli.Command) (context.Context, error) {
	return func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Args().Len() < n {
			cli.ShowSubcommandHelpAndExit(cmd, exitUserError)
		}
		return ctx, nil
	}
}

func requireMaxArgs(n int) func(context.Context, *cli.Command) (context.Context, error) {
	return func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Args().Len() > n {
			cli.ShowSubcommandHelpAndExit(cmd, exitUserError)
		}
		return ctx, nil
	}
}

func requireArgs(n int) func(context.Context, *cli.Command) (context.Context, error) {
	return func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Args().Len() != n {
			cli.ShowSubcommandHelpAndExit(cmd, exitUserError)
		}
		return ctx, nil
	}
}

func requireArgsRange(lo, hi int) func(context.Context, *cli.Command) (context.Context, error) {
	return func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		n := cmd.Args().Len()
		if n < lo || n > hi {
			cli.ShowSubcommandHelpAndExit(cmd, exitUserError)
		}
		return ctx, nil
	}
}

// Flag parsing
// -----------------------------------------------------------------------------

func parseColorMode(cmd *cli.Command) (signal.ColorMode, error) {
	s := cmd.String(flagColor)

	switch strings.ToLower(s) {
	case "auto":
		return signal.ColorAuto, nil
	case "always":
		return signal.ColorAlways, nil
	case "never":
		return signal.ColorNever, nil
	default:
		// bare-error: CLI flag validation, not reachable through engine
		return 0, errs.Errorf("invalid --color value %q (expected auto, always, or never)", s)
	}
}

func parseVerbosity(cmd *cli.Command) signal.Verbosity {
	v := cmd.Count(flagVerbosity)

	switch {
	case v >= 3:
		return signal.VVV
	case v == 2:
		return signal.VV
	case v == 1:
		return signal.V
	default:
		return signal.Quiet
	}
}

func mustGlobalOpts(ctx context.Context) globalOpts {
	return ctx.Value(ctxGlobalOpts).(globalOpts)
}

// Displayer
// -----------------------------------------------------------------------------

func newDisplayer(opts globalOpts, store *diagnostic.SourceStore) diagnostic.Displayer {
	d := clir.New(
		clir.Options{
			ColorMode:  opts.colorMode,
			Verbosity:  opts.verbosity,
			ForceASCII: opts.ascii,
		},
		store,
	)
	interruptMu.Lock()
	interruptHook = d.Interrupt
	interruptMu.Unlock()
	return d
}

// withDisplayer creates a displayer and returns a cleanup function that
// should be deferred. The cleanup function closes the displayer and
// recovers from panics.
func withDisplayer(opts globalOpts, store *diagnostic.SourceStore) (diagnostic.Displayer, func()) {
	d := newDisplayer(opts, store)
	return d, func() {
		d.Close()
		recoverAndReport(recover())
	}
}

// cliPolicy returns the standard diagnostic policy for CLI commands:
// caller verbosity, dedup enabled, no warnings-as-errors. Special
// cases (inspect's --diff suppressing plan output) mutate the
// returned value before passing it to diagnostic.NewEmitter.
func cliPolicy(opts globalOpts) diagnostic.Policy {
	return diagnostic.Policy{
		Verbosity:        opts.verbosity,
		DedupDiagnostics: true,
	}
}

// Resolve options
// -----------------------------------------------------------------------------

func resolveFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  flagOnly,
			Usage: "filter to specific deploy blocks (comma-separated)",
		},
		&cli.StringFlag{
			Name:  flagTargets,
			Usage: "filter to specific targets (comma-separated)",
		},
	}
}

func parseResolveOpts(cmd *cli.Command) spec.ResolveOptions {
	opts := spec.ResolveOptions{}

	if s := cmd.String(flagOnly); s != "" {
		opts.DeployNames = splitComma(s)
	}
	if s := cmd.String(flagTargets); s != "" {
		opts.TargetNames = splitComma(s)
	}

	return opts
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// Error handling
// -----------------------------------------------------------------------------

func handleEngineError(name string, err error) error {
	if err == nil {
		return nil
	}
	var abort engine.AbortError
	if errors.As(err, &abort) {
		return cli.Exit("", exitUserError)
	}
	var cancelled engine.CancelledError
	if errors.As(err, &cancelled) {
		return cli.Exit("", exitUserError)
	}
	panic(errs.BUG("engine.%s returned unexpected error: %w", name, err))
}

func recoverAndReport(r any) {
	if r == nil {
		return
	}

	// Always write to stderr, ignore all errors.
	// what should we do in a panic handler? die again? please.
	_println := func(a ...any) {
		_, _ = fmt.Fprintln(os.Stderr, a...)
	}

	_println()
	_println("[scampi] fatal internal error")
	_println()
	_println("This is a BUG in scampi, not in your configuration.")
	_println()
	_println("Please report this issue and include the information below:")
	_println("  https://codeberg.org/scampi-dev/scampi/issues")
	_println()
	_println()
	_println("======  internal error  ======")

	switch v := r.(type) {
	case error:
		_println(v.Error())
	default:
		_println(v)
	}

	_println()
	_println("======    stack trace   ======")
	_println(string(debug.Stack()))

	// Hard exit with a distinct code for internal bugs
	os.Exit(exitBug)
}
