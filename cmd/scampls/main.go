// SPDX-License-Identifier: GPL-3.0-only

// scampls is the Language Server Protocol server for scampi
// configuration files. It communicates over stdin/stdout using the
// standard LSP JSON-RPC transport.
//
// Usage:
//
//	scampls [--log FILE]      # serve LSP over stdio (default)
//	scampls check FILE...     # one-shot diagnostic on file(s)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"

	"github.com/urfave/cli/v3"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/lsp"
)

var version = "v0.0.0-dev"

func main() {
	app := &cli.Command{
		Name:    "scampls",
		Usage:   "Language Server Protocol server for scampi configs",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "log",
				Usage: "write debug log to file",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			lsp.Version = version

			var opts []lsp.Option
			if logPath := cmd.String("log"); logPath != "" {
				f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
				if err != nil {
					// bare-error: CLI boundary, not reachable through engine
					return errs.Errorf("open log: %w", err)
				}
				defer func() { _ = f.Close() }()
				opts = append(opts, lsp.WithLog(
					log.New(f, "scampls: ", log.Ltime|log.Lmicroseconds),
				))
			}

			return lsp.Serve(ctx, os.Stdin, os.Stdout, opts...)
		},
		Commands: []*cli.Command{
			{
				Name:      "check",
				Usage:     "run LSP diagnostic pipeline on file(s) and print results",
				ArgsUsage: "FILE...",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if len(args) == 0 {
						// bare-error: CLI boundary
						return errs.New("check: at least one FILE required")
					}
					anyError := false
					for _, path := range args {
						res := lsp.CheckFile(ctx, path)
						if res.Panic != "" {
							_, _ = fmt.Fprintf(os.Stderr, "PANIC checking %s:\n%s\n", path, res.Panic)
							anyError = true
							continue
						}
						if out := lsp.FormatDiagnostics(path, res.Diagnostics); out != "" {
							_, _ = fmt.Fprint(os.Stdout, out)
							anyError = true
						}
					}
					if anyError {
						os.Exit(1)
					}
					return nil
				},
			},
			{
				Name:      "hover",
				Usage:     "run an LSP hover request at FILE:LINE:COL and print the result",
				ArgsUsage: "FILE LINE COL",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if len(args) != 3 {
						// bare-error: CLI boundary
						return errs.New("hover: usage: hover FILE LINE COL")
					}
					line, err := strconv.ParseUint(args[1], 10, 32)
					if err != nil {
						// bare-error: CLI boundary
						return errs.Errorf("hover: bad LINE: %w", err)
					}
					col, err := strconv.ParseUint(args[2], 10, 32)
					if err != nil {
						// bare-error: CLI boundary
						return errs.Errorf("hover: bad COL: %w", err)
					}
					res := lsp.HoverFile(ctx, args[0], uint32(line), uint32(col))
					if res.Panic != "" {
						_, _ = fmt.Fprintf(os.Stderr, "PANIC:\n%s\n", res.Panic)
						os.Exit(1)
					}
					if res.Markdown == "" {
						_, _ = fmt.Fprintln(os.Stdout, "(no hover info)")
						return nil
					}
					_, _ = fmt.Fprintln(os.Stdout, res.Markdown)
					return nil
				},
			},
			{
				Name:      "def",
				Usage:     "run an LSP goto-definition request at FILE:LINE:COL and print the result",
				ArgsUsage: "FILE LINE COL",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if len(args) != 3 {
						// bare-error: CLI boundary
						return errs.New("def: usage: def FILE LINE COL")
					}
					line, err := strconv.ParseUint(args[1], 10, 32)
					if err != nil {
						// bare-error: CLI boundary
						return errs.Errorf("def: bad LINE: %w", err)
					}
					col, err := strconv.ParseUint(args[2], 10, 32)
					if err != nil {
						// bare-error: CLI boundary
						return errs.Errorf("def: bad COL: %w", err)
					}
					res := lsp.DefFile(ctx, args[0], uint32(line), uint32(col))
					if res.Panic != "" {
						_, _ = fmt.Fprintf(os.Stderr, "PANIC:\n%s\n", res.Panic)
						os.Exit(1)
					}
					if len(res.Locations) == 0 {
						_, _ = fmt.Fprintln(os.Stdout, "(no definition)")
						return nil
					}
					for _, loc := range res.Locations {
						_, _ = fmt.Fprintf(
							os.Stdout,
							"%s:%d:%d\n",
							loc.URI,
							loc.Range.Start.Line+1,
							loc.Range.Start.Character+1,
						)
					}
					return nil
				},
			},
			{
				Name:      "scan",
				Usage:     "run cursor-driven LSP requests at every position in FILE; report crashes",
				ArgsUsage: "FILE",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if len(args) != 1 {
						// bare-error: CLI boundary
						return errs.New("scan: exactly one FILE required")
					}
					res, err := lsp.ScanFile(ctx, args[0])
					if err != nil {
						// bare-error: CLI boundary
						return errs.Errorf("scan: %w", err)
					}
					if len(res.Crashes) == 0 {
						_, _ = fmt.Fprintln(os.Stdout, "no crashes")
						return nil
					}
					for _, c := range res.Crashes {
						_, _ = fmt.Fprintf(
							os.Stderr,
							"PANIC %s at %s:%d:%d\n%s\n\n",
							c.Request,
							args[0],
							c.Line,
							c.Col,
							c.Panic,
						)
					}
					_, _ = fmt.Fprintf(os.Stderr, "%d crash(es) total\n", len(res.Crashes))
					os.Exit(1)
					return nil
				},
			},
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := app.Run(ctx, os.Args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "scampls: %s\n", err)
		os.Exit(1)
	}
}
