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

	"scampi.dev/scampi/internal/lang/format"
)

func fmtCmd() *cli.Command {
	return &cli.Command{
		Name:      "fmt",
		Usage:     "Format scampi files",
		ArgsUsage: "<files or directories...>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "list",
				Aliases: []string{"l"},
				Usage:   "list files that would be reformatted (don't write)",
			},
		},
		Before: requireMinArgs(1),
		Action: func(_ context.Context, cmd *cli.Command) error {
			listOnly := cmd.Bool("list")
			args := cmd.Args().Slice()

			var files []string
			for _, arg := range args {
				recursive := false
				if strings.HasSuffix(arg, "/...") {
					arg = strings.TrimSuffix(arg, "/...")
					if arg == "" {
						arg = "."
					}
					recursive = true
				}
				info, err := os.Stat(arg)
				if err != nil {
					return cliError(fmt.Sprintf("cannot access %s: %s", arg, err))
				}
				if info.IsDir() {
					if recursive {
						found, err := findScampiFilesRecursive(arg)
						if err != nil {
							return cliError(fmt.Sprintf("walking %s: %s", arg, err))
						}
						files = append(files, found...)
					} else {
						found, err := findScampiFilesFlat(arg)
						if err != nil {
							return cliError(fmt.Sprintf("reading %s: %s", arg, err))
						}
						files = append(files, found...)
					}
				} else {
					files = append(files, arg)
				}
			}

			changed := 0
			for _, path := range files {
				src, err := os.ReadFile(path)
				if err != nil {
					return cliError(fmt.Sprintf("reading %s: %s", path, err))
				}
				out, err := format.Format(src)
				if err != nil {
					return cliError(fmt.Sprintf("formatting %s: %s", path, err))
				}
				if bytes.Equal(src, out) {
					continue
				}
				changed++
				if listOnly {
					_, _ = fmt.Println(path)
					continue
				}
				if err := os.WriteFile(path, out, 0o644); err != nil {
					return cliError(fmt.Sprintf("writing %s: %s", path, err))
				}
				_, _ = fmt.Println(path)
			}

			if listOnly && changed > 0 {
				return cli.Exit("", 1)
			}
			return nil
		},
	}
}

func findScampiFilesRecursive(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".scampi") && !strings.HasSuffix(path, "_test.scampi") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func findScampiFilesFlat(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".scampi") && !strings.HasSuffix(e.Name(), "_test.scampi") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}
