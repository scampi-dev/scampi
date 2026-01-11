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
)

const (
	exitOK        = 0 // success
	exitUserError = 1 // invalid config, failed plan, validation errors
	exitBug       = 2 // internal error / panic
)

func main() {
	doit := &cli.Command{
		Name:  "doit",
		Usage: "Declarative task execution for local and remote systems",
		Commands: []*cli.Command{
			applyCmd(),
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
	var colorFlag string
	var verbosity int

	return &cli.Command{
		Name:                   "apply",
		Usage:                  "Apply the desired state defined in a configuration file",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "color",
				Value:       "auto",
				Usage:       "colorize output: auto, always, never",
				Destination: &colorFlag,
			},
			&cli.BoolFlag{
				Name:  "v",
				Usage: "increase verbosity (-v, -vv, -vvv, -vvvv)",
				Config: cli.BoolConfig{
					Count: &verbosity,
				},
			},
		},
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			colorMode, err := parseColorMode(colorFlag)
			if err != nil {
				return cli.Exit(err.Error(), exitUserError)
			}

			v := mapVerbosity(verbosity)

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        v,
			}

			store := spec.NewSourceStore()

			displ := render.NewCLI(
				render.CLIOptions{
					ColorMode: colorMode,
					Verbosity: v,
				},
				store)

			defer func() {
				displ.Close()
				recoverAndReport(recover())
			}()

			em := diagnostic.NewEmitter(pol, displ)
			err = engine.Apply(ctx, em, cfg, store)
			if err != nil {
				var abort engine.AbortError
				if errors.As(err, &abort) {
					return cli.Exit("", exitUserError)
				}

				// Engine violated its contract: unexpected error
				panic(fmt.Errorf("BUG: engine.Apply returned unexpected error: %w", err))
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
			return ctx, cli.Exit("", 1)
		}
		return ctx, nil
	}
}

func parseColorMode(s string) (signal.ColorMode, error) {
	switch strings.ToLower(s) {
	case "auto":
		return signal.ColorAuto, nil
	case "always":
		return signal.ColorAlways, nil
	case "never":
		return signal.ColorNever, nil
	default:
		return 0, fmt.Errorf("invalid --color value %q (expected auto, always, or never)", s)
	}
}

func mapVerbosity(v int) signal.Verbosity {
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
