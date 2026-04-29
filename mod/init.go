// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
)

// Init creates a scampi.mod file in dir with the given module path.
// If modulePath is empty, it is inferred from the git remote origin URL.
func Init(ctx context.Context, src source.Source, dir string, modulePath string) error {
	dest := filepath.Join(dir, "scampi.mod")
	span := spec.SourceSpan{Filename: dest}

	if modulePath == "" {
		inferred, err := inferModulePath(dir, span)
		if err != nil {
			return err
		}
		modulePath = inferred
	}

	if !isModulePath(modulePath) {
		return &InitError{
			Detail: fmt.Sprintf("invalid module path %q", modulePath),
			Hint:   "module path must be a host/path URL, e.g. codeberg.org/yourname/yourmodule",
			Source: span,
		}
	}

	meta, err := src.Stat(ctx, dest)
	if err != nil {
		return &InitStatError{Path: dest, Cause: err}
	}
	if meta.Exists {
		return &InitError{
			Detail: "scampi.mod already exists",
			Hint:   "delete it first or edit it directly",
			Source: span,
		}
	}

	content := "module " + modulePath + "\n"
	if err := src.WriteFile(ctx, dest, []byte(content)); err != nil {
		return &InitError{
			Detail: fmt.Sprintf("could not write scampi.mod: %v", err),
			Hint:   "check directory permissions",
			Source: span,
		}
	}

	return nil
}

func inferModulePath(dir string, span spec.SourceSpan) (string, error) {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", &InitError{
			Detail: "could not infer module path from git remote",
			Hint:   "specify it explicitly: scampi mod init <module-path>",
			Source: span,
		}
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return "", &InitError{
			Detail: "git remote origin is empty",
			Hint:   "specify it explicitly: scampi mod init <module-path>",
			Source: span,
		}
	}
	return urlToModulePath(raw), nil
}

func urlToModulePath(raw string) string {
	for _, prefix := range []string{"https://", "http://", "git://"} {
		if after, ok := strings.CutPrefix(raw, prefix); ok {
			raw = after
			break
		}
	}

	if after, ok := strings.CutPrefix(raw, "git@"); ok {
		raw = strings.Replace(after, ":", "/", 1)
	}

	raw = strings.TrimSuffix(raw, ".git")
	raw = strings.TrimRight(raw, "/")

	return raw
}
