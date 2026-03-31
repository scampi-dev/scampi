// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"scampi.dev/scampi/source"
)

var loadPathRe = regexp.MustCompile(`load\(\s*"([^"]+)"`)

// Tidy scans *.scampi files in dir for load() calls and synchronises the
// require block in scampi.mod to match. It returns a list of human-readable
// change descriptions, or nil if nothing changed.
func Tidy(ctx context.Context, src source.Source, dir string) ([]string, error) {
	modPath := filepath.Join(dir, "scampi.mod")
	data, err := src.ReadFile(ctx, modPath)
	if err != nil {
		return nil, &TidyError{
			Detail: fmt.Sprintf("could not read scampi.mod: %v", err),
			Hint:   "run: scampi mod init",
		}
	}

	mod, err := Parse(modPath, data)
	if err != nil {
		return nil, err
	}

	refs, err := collectLoadPaths(ctx, src, dir, mod)
	if err != nil {
		return nil, err
	}

	existing := make(map[string]string, len(mod.Require))
	for _, dep := range mod.Require {
		existing[dep.Path] = dep.Version
	}

	var toAdd []string
	for path := range refs {
		if _, ok := existing[path]; !ok {
			toAdd = append(toAdd, path)
		}
	}
	slices.Sort(toAdd)

	var toRemove []string
	for path := range existing {
		if !refs[path] {
			toRemove = append(toRemove, path)
		}
	}
	slices.Sort(toRemove)

	if len(toAdd) == 0 && len(toRemove) == 0 {
		return nil, nil
	}

	var newDeps []Dependency
	for _, dep := range mod.Require {
		if refs[dep.Path] {
			newDeps = append(newDeps, Dependency{Path: dep.Path, Version: dep.Version})
		}
	}
	for _, path := range toAdd {
		newDeps = append(newDeps, Dependency{Path: path, Version: "v0.0.0"})
	}
	slices.SortFunc(newDeps, func(a, b Dependency) int {
		return strings.Compare(a.Path, b.Path)
	})

	if err := WriteModFile(ctx, src, modPath, mod.Module, newDeps); err != nil {
		return nil, err
	}

	var changes []string
	for _, path := range toAdd {
		changes = append(changes, "added "+path+" v0.0.0")
	}
	for _, path := range toRemove {
		changes = append(changes, "removed "+path)
	}
	return changes, nil
}

func collectLoadPaths(ctx context.Context, src source.Source, dir string, mod *Module) (map[string]bool, error) {
	// filepath.Glob lists filenames (metadata only). source.Source doesn't
	// have a directory-listing method, so we keep this for now.
	pattern := filepath.Join(dir, "*.scampi")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, &TidyError{
			Detail: fmt.Sprintf("could not scan *.scampi files: %v", err),
			Hint:   "check directory permissions",
		}
	}

	refs := map[string]bool{}
	for _, f := range files {
		data, err := src.ReadFile(ctx, f)
		if err != nil {
			return nil, &TidyError{
				Detail: fmt.Sprintf("could not read %s: %v", f, err),
				Hint:   "check file permissions",
			}
		}

		for _, match := range loadPathRe.FindAllSubmatch(data, -1) {
			raw := string(match[1])
			mp := extractModulePath(raw, mod)
			if mp != "" {
				refs[mp] = true
			}
		}
	}
	return refs, nil
}

func extractModulePath(raw string, mod *Module) string {
	if !isModulePath(raw) {
		return ""
	}

	if mod != nil {
		dep, _ := splitModulePath(mod, raw)
		if dep != nil {
			return dep.Path
		}
	}

	parts := strings.SplitN(raw, "/", 4)
	if len(parts) >= 3 {
		return strings.Join(parts[:3], "/")
	}
	return raw
}
