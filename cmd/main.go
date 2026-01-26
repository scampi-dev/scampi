// Package main defines the CLI surface of doit.
//
// It wires user-facing commands to engine execution, diagnostics, and rendering.
// This package contains no execution semantics; it is responsible only for
// argument parsing, command dispatch, and process exit behavior.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	ossig "os/signal"
	"runtime/debug"
	"strings"

	"github.com/urfave/cli/v3"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/osutil"
	"godoit.dev/doit/render"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/util"
)

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

	ctxGlobalOpts = ctxKey("globalOpts")
)

func main() {
	doit := &cli.Command{
		Name:  "doit",
		Usage: "Declarative task execution for local and remote systems",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  flagASCII,
				Usage: "force ASCII output (disable fancy glyphs)",
			},
			&cli.StringFlag{
				Name:  flagColor,
				Value: "auto",
				Usage: "colorize output: auto, always, never",
			},
			&cli.BoolFlag{
				Name:  flagVerbosity,
				Usage: "increase verbosity (-v, -vv, -vvv, -vvvv)",
			},
		},
		Commands: []*cli.Command{
			applyCmd(),
			checkCmd(),
			planCmd(),
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

	ctx, stop := ossig.NotifyContext(
		context.Background(),
		osutil.MainContextSignals...,
	)
	defer stop()

	if err := doit.Run(ctx, os.Args); err != nil {
		// must never happen, since we either return cleanly
		// or we handle abort and other errors ourselves with exit-codes
		log.Fatalf("unhandled error: %#v\n", err)
	}
}

func applyCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:                   "apply",
		Usage:                  "Apply the desired state defined in a configuration file",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file and executes
the required actions to converge the system into the desired state.

The command is idempotent: running it multiple times will only apply
changes when the current state differs from the declared state.`,
		Before: requireArgs(1),
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
			}

			store := spec.NewSourceStore()

			displ := newDisplayer(opts, store)
			defer func() {
				displ.Close()
				recoverAndReport(recover())
			}()

			em := diagnostic.NewEmitter(pol, displ)
			err := engine.Apply(ctx, em, cfg, store)
			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					// Engine violated its contract: unexpected error
					panic(util.BUG("engine.Apply returned unexpected error: %w", err))
				}

				return cli.Exit("", exitUserError)
			}

			return nil
		},
	}
}

func checkCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:                   "check",
		Usage:                  "Check what would change without applying",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file and checks what
would need to change to converge the system into the desired state.

Unlike 'plan', this command inspects the target system to determine
which operations are already satisfied and which would need to execute.
No changes are made to the system.`,
		Before: requireArgs(1),
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
			}

			store := spec.NewSourceStore()

			displ := newDisplayer(opts, store)
			defer func() {
				displ.Close()
				recoverAndReport(recover())
			}()

			em := diagnostic.NewEmitter(pol, displ)
			err := engine.Check(ctx, em, cfg, store)
			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					// Engine violated its contract: unexpected error
					panic(util.BUG("engine.Check returned unexpected error: %w", err))
				}

				return cli.Exit("", exitUserError)
			}

			return nil
		},
	}
}

func planCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:                   "plan",
		Usage:                  "Show the execution plan for a configuration file",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file and prints the
execution plan without applying any changes.

The plan shows the actions and operations that would be executed by
the apply command, but does not modify the system.`,
		Before: requireArgs(1),
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
			}

			store := spec.NewSourceStore()

			displ := newDisplayer(opts, store)
			defer func() {
				displ.Close()
				recoverAndReport(recover())
			}()

			em := diagnostic.NewEmitter(pol, displ)

			err := engine.Plan(ctx, em, cfg, store)
			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					// Engine violated its contract: unexpected error
					panic(util.BUG("engine.Apply returned unexpected error: %w", err))
				}

				return cli.Exit("", exitUserError)
			}

			return nil
		},
	}
}

func requireArgs(n int) func(context.Context, *cli.Command) (context.Context, error) {
	return func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Args().Len() != n {
			if err := cli.ShowSubcommandHelp(cmd); err != nil {
				return ctx, err
			}
			return ctx, cli.Exit("", exitUserError)
		}
		return ctx, nil
	}
}

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
		return 0, util.Errorf("invalid --color value %q (expected auto, always, or never)", s)
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

func newDisplayer(opts globalOpts, store *spec.SourceStore) render.Displayer {
	return render.NewCLI(
		render.CLIOptions{
			ColorMode:  opts.colorMode,
			Verbosity:  opts.verbosity,
			ForceASCII: opts.ascii,
		},
		store,
	)
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
	_println("[doit] fatal internal error")
	_println()
	_println("This is a BUG in doit, not in your configuration.")
	_println()
	_println("Please report this issue and include the information below:")
	_println("  https://godoit.dev/issues/new")
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
