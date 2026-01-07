package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/render"
)

func main() {
	ctx := context.Background()

	doit := &cli.Command{
		Name:  "doit",
		Usage: "Declarative task execution for local and remote systems",
		Commands: []*cli.Command{
			applyCmd(),
		},
	}

	if err := doit.Run(ctx, os.Args); err != nil {
		log.Fatal(err)
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
				return err
			}

			pol := diagnostic.Policy{
				WarningsAsErrors: false,
				Verbosity:        mapVerbosity(verbosity),
			}

			displ := render.NewCLI(render.CLIOptions{
				ColorMode: colorMode,
			})

			em := diagnostic.NewEmitter(pol, displ)

			return engine.Apply(ctx, em, cfg)
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

func parseColorMode(s string) (render.ColorMode, error) {
	switch strings.ToLower(s) {
	case "auto":
		return render.ColorAuto, nil
	case "always":
		return render.ColorAlways, nil
	case "never":
		return render.ColorNever, nil
	default:
		return 0, fmt.Errorf("invalid --color value %q (expected auto, always, or never)", s)
	}
}

func mapVerbosity(v int) diagnostic.Verbosity {
	switch {
	case v >= 3:
		return diagnostic.DebugVerbose
	case v == 2:
		return diagnostic.VeryVerbose
	case v == 1:
		return diagnostic.Verbose
	default:
		return diagnostic.Quiet
	}
}
