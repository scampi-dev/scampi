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

	"github.com/urfave/cli/v3"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/osutil"
	"scampi.dev/scampi/render"
	clir "scampi.dev/scampi/render/cli"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

var version = "dev"

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
		Name:        "scampi",
		Usage:       "Declarative task execution for local and remote systems",
		Version:     version,
		HideVersion: true,
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
						return fmt.Errorf("invalid --color value %q (expected auto, always, or never)", s)
					}
				},
			},
			&cli.BoolFlag{
				Name:  flagVerbosity,
				Usage: "increase verbosity (-v, -vv, -vvv, -vvvv)",
			},
		},
		Commands: []*cli.Command{
			applyCmd(),
			checkCmd(),
			inspectCmd(),
			planCmd(),
			indexCmd(),
			legendCmd(),
			secretsCmd(),
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

	ctx, stop := ossig.NotifyContext(
		context.Background(),
		osutil.MainContextSignals...,
	)
	defer stop()

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

func applyCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:                   "apply",
		Usage:                  "Apply the desired state from a configuration file",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Flags:                  resolveFlags(),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file and executes the
required operations to converge the system to the desired state.

The command is idempotent: running it multiple times only applies
changes when the current state differs from the declared state.`,
		Before: requireArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
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

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)
			err := engine.Apply(ctx, em, cfg, store, resolveOpts)
			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					// Engine violated its contract: unexpected error
					panic(errs.BUG("engine.Apply returned unexpected error: %w", err))
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
		Usage:                  "Check the current system state against a configuration file",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Flags:                  resolveFlags(),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file and inspects the
target system to determine which operations are already satisfied and
which would need to execute.

No changes are made to the system. Unlike 'plan', this command evaluates
the actual system state.`,
		Before: requireArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
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

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)
			err := engine.Check(ctx, em, cfg, store, resolveOpts)
			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					// Engine violated its contract: unexpected error
					panic(errs.BUG("engine.Check returned unexpected error: %w", err))
				}

				return cli.Exit("", exitUserError)
			}

			return nil
		},
	}
}

func inspectCmd() *cli.Command {
	var cfg string

	return &cli.Command{
		Name:                   "inspect",
		Usage:                  "Compare desired file content against the current target state",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Flags: append(resolveFlags(), &cli.StringFlag{
			Name:  "step",
			Usage: "filter to a specific file op by destination path (substring match)",
		}),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "config",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &cfg,
			},
		},
		ArgsUsage: "<config>",
		Description: `Reads a declarative configuration file, extracts file content
from inspectable ops (e.g. copy), and opens a diff tool to compare
the desired state against what currently exists on the target.

Set SCAMPI_DIFFTOOL, DIFFTOOL, or EDITOR to choose your diff tool.
Falls back to plain diff(1).`,
		Before: requireArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
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

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)

			stepFilter := cmd.String("step")
			result, err := engine.Inspect(ctx, em, cfg, store, resolveOpts, stepFilter)
			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					panic(errs.BUG("engine.Inspect returned unexpected error: %w", err))
				}

				return cli.Exit("", exitUserError)
			}

			tool := osutil.ResolveDiffTool()
			return osutil.RunDiffTool(ctx, tool, result.DestPath, result.Current, result.Desired)
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
		OnUsageError:           onUsageError,
		Flags:                  resolveFlags(),
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

The plan shows the operations that would be executed by 'apply', but
does not inspect or modify the target system.`,
		Before: requireArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
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

			resolveOpts := parseResolveOpts(cmd)
			em := diagnostic.NewEmitter(pol, displ)

			err := engine.Plan(ctx, em, cfg, store, resolveOpts)
			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					// Engine violated its contract: unexpected error
					panic(errs.BUG("engine.Plan returned unexpected error: %w", err))
				}

				return cli.Exit("", exitUserError)
			}

			return nil
		},
	}
}

func indexCmd() *cli.Command {
	return &cli.Command{
		Name:                   "index",
		Usage:                  "List available steps and their documentation",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		ArgsUsage:              "[step]",
		Description: `Prints the index of steps supported by the engine.

Without arguments, the command lists all available steps with a short
description. When a step name is provided, detailed documentation is
shown, including fields, behavior, and examples.`,
		Before: requireMaxArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        opts.verbosity,
			}

			displ := newDisplayer(opts, nil)
			defer func() {
				displ.Close()
				recoverAndReport(recover())
			}()

			em := diagnostic.NewEmitter(pol, displ)
			args := cmd.Args()

			var err error
			if args.Len() == 0 {
				err = engine.IndexAll(ctx, em)
			} else {
				err = engine.IndexStep(ctx, args.First(), em)
			}

			if err != nil {
				var abort engine.AbortError
				if !errors.As(err, &abort) {
					// Engine violated its contract: unexpected error
					panic(errs.BUG("engine.Index returned unexpected error: %w", err))
				}

				return cli.Exit("", exitUserError)
			}

			return nil
		},
	}
}

func legendCmd() *cli.Command {
	return &cli.Command{
		Name:  "legend",
		Usage: "Show the CLI visual language reference",
		Description: `Prints a reference card for glyphs, plan structure,
and color semantics used in scampi CLI output.`,
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)
			displ := newDisplayer(opts, nil)
			defer displ.Close()
			displ.EmitLegend()
			return nil
		},
	}
}

func onUsageError(_ context.Context, cmd *cli.Command, err error, _ bool) error {
	_, _ = fmt.Fprintf(os.Stderr, "Incorrect Usage: %s\n\n", err)
	_ = cli.ShowSubcommandHelp(cmd)
	return cli.Exit("", exitUserError)
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

func newDisplayer(opts globalOpts, store *spec.SourceStore) render.Displayer {
	return clir.New(
		clir.Options{
			ColorMode:  opts.colorMode,
			Verbosity:  opts.verbosity,
			ForceASCII: opts.ascii,
		},
		store,
	)
}

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

func versionCmd() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print the scampi version",
		Action: func(_ context.Context, _ *cli.Command) error {
			_, _ = fmt.Println("scampi " + version)
			return nil
		},
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
