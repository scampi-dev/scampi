// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/testkit"
)

func testCmd() *cli.Command {
	var testPath string

	return &cli.Command{
		Name:         "test",
		Usage:        "Run scampi test files",
		ArgsUsage:    "[path]",
		OnUsageError: onUsageError,
		Before:       requireMaxArgs(1),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "path",
				Config:      cli.StringConfig{TrimSpace: true},
				Destination: &testPath,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			opts := mustGlobalOpts(ctx)
			store := diagnostic.NewSourceStore()
			displ, cleanup := withDisplayer(opts, store)
			defer cleanup()

			pol := diagnostic.Policy{Verbosity: opts.verbosity}
			em := diagnostic.NewEmitter(pol, displ)
			src := source.LocalPosixSource{}

			files, err := findTestFiles(testPath)
			if err != nil {
				emitTestDiag(em, &testkit.TestError{
					Detail: err.Error(),
					Hint:   "test files must end in _test.scampi",
				})
				return cli.Exit("", exitUserError)
			}
			if len(files) == 0 {
				emitTestDiag(em, &testkit.TestError{
					Detail: "no test files found",
					Hint:   "test files must end in _test.scampi",
				})
				return cli.Exit("", exitUserError)
			}

			totalPassed, totalFailed := 0, 0

			for _, f := range files {
				passed, failed, err := runLangTestFile(ctx, em, f, src)
				if err != nil {
					emitTestDiag(em, &testkit.TestError{
						Detail: err.Error(),
						Hint:   "fix the test file",
					})
					totalFailed++
					continue
				}
				totalPassed += passed
				totalFailed += failed

				emitTestInfo(em, f, &testkit.TestSummary{
					Passed: passed,
					Failed: failed,
					File:   f,
				})
			}

			if totalFailed > 0 {
				return cli.Exit("", exitUserError)
			}
			return nil
		},
	}
}

// findTestFiles resolves the test path argument into a list of *_test.scampi files.
//
//   - ""             → *_test.scampi in current dir
//   - "./..."        → recursive from current dir
//   - "path/..."     → recursive from path
//   - "path/to/dir"  → *_test.scampi in that dir
//   - "file.scampi"    → that specific file
func findTestFiles(arg string) ([]string, error) {
	if arg == "" {
		return filepath.Glob("*_test.scampi")
	}

	if strings.HasSuffix(arg, "/...") || arg == "./..." {
		root := strings.TrimSuffix(arg, "/...")
		if root == "." || root == "" {
			root = "."
		}
		return walkTestFiles(root)
	}

	info, err := os.Stat(arg)
	if err == nil && info.IsDir() {
		return filepath.Glob(filepath.Join(arg, "*_test.scampi"))
	}

	return []string{arg}, nil
}

func walkTestFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), "_test.scampi") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func emitTestDiag(em diagnostic.Emitter, d diagnostic.Diagnostic) {
	em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic("", d))
}

func emitTestInfo(em diagnostic.Emitter, path string, d diagnostic.Diagnostic) {
	em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic(path, d))
}
