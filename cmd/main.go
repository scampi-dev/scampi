package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"
	"godoit.dev/doit/engine"
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

	return &cli.Command{
		Name:  "apply",
		Usage: "Apply the desired state defined in a configuration file",
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
			return engine.Apply(ctx, cfg)
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
