// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
	"scampi.dev/scampi/diagnostic"
	scampigen "scampi.dev/scampi/gen"
)

// scampi gen
// -----------------------------------------------------------------------------

func genCmd() *cli.Command {
	return &cli.Command{
		Name:                   "gen",
		Usage:                  "Generate scampi modules from external schemas",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Commands: []*cli.Command{
			genAPICmd(),
		},
	}
}

// scampi gen api
// -----------------------------------------------------------------------------

func genAPICmd() *cli.Command {
	var specPath, output, prefix, namePrefix, moduleName string
	var noTest bool

	return &cli.Command{
		Name:                   "api",
		Usage:                  "Generate .api.scampi module from an OpenAPI specification",
		ArgsUsage:              "<spec.yaml>",
		UseShortOptionHandling: true,
		Suggest:                true,
		HideHelp:               false,
		OnUsageError:           onUsageError,
		Before:                 requireArgs(1),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Usage:       "output file path (derives from spec name by default)",
				Destination: &output,
			},
			&cli.StringFlag{
				Name:        "prefix",
				Aliases:     []string{"p"},
				Usage:       "path prefix prepended to all generated routes (e.g. /integration)",
				Destination: &prefix,
			},
			&cli.StringFlag{
				Name:        "name-prefix",
				Aliases:     []string{"n"},
				Usage:       "prefix prepended to all generated function names (e.g. legacy_)",
				Destination: &namePrefix,
			},
			&cli.StringFlag{
				Name:        "module",
				Aliases:     []string{"m"},
				Usage:       "override the module declaration name (default: derived from spec filename)",
				Destination: &moduleName,
			},
			&cli.BoolFlag{
				Name:        "no-test",
				Usage:       "skip generating the companion smoke test file",
				Destination: &noTest,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "spec",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &specPath,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)

			displ, cleanup := withDisplayer(opts, nil)
			defer cleanup()

			pol := diagnostic.Policy{
				Verbosity: opts.verbosity,
			}
			em := diagnostic.NewEmitter(pol, displ)

			outPath := output
			if outPath == "" {
				base := strings.TrimSuffix(specPath, filepath.Ext(specPath))
				outPath = base + ".api.scampi"
			}

			genOpts := scampigen.APIOptions{
				PathPrefix: prefix,
				NamePrefix: namePrefix,
				ModuleName: moduleName,
				NoTest:     noTest,
			}

			if outPath == "-" {
				return handleEngineError(
					"gen api",
					scampigen.API(specPath, version, os.Stdout, em, genOpts),
				)
			}

			f, err := os.Create(outPath)
			if err != nil {
				return cli.Exit(err.Error(), exitUserError)
			}
			defer func() { _ = f.Close() }()

			// Buffer test output — only write to disk if generation succeeds.
			var testBuf bytes.Buffer
			var testPath string
			if !noTest {
				base := strings.TrimSuffix(outPath, ".api.scampi")
				if base == outPath {
					base = strings.TrimSuffix(outPath, filepath.Ext(outPath))
				}
				testPath = base + "_test.scampi"
				genOpts.TestWriter = &testBuf
			}

			if err := scampigen.API(specPath, version, f, em, genOpts); err != nil {
				_ = os.Remove(outPath)
				return handleEngineError("gen api", err)
			}

			_, _ = fmt.Fprintf(os.Stderr, "wrote %s\n", outPath)

			if testPath != "" && testBuf.Len() > 0 {
				if writeErr := os.WriteFile(testPath, testBuf.Bytes(), 0o644); writeErr == nil {
					_, _ = fmt.Fprintf(os.Stderr, "wrote %s\n", testPath)
				}
			}
			return nil
		},
	}
}
