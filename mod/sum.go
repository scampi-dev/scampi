// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
)

// ComputeHash computes a deterministic SHA-256 hash of the directory tree at dir.
// It skips .git/ directories, sorts paths lexicographically, and encodes each
// file as "<forward-slash-path>\0<content>" before hashing.
// Returns "h1:" + hex(sha256).
//
// This operates on the module cache (real filesystem), not project source files.
func ComputeHash(dir string) (string, error) {
	var paths []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", &SumError{
			Detail: "failed to walk directory: " + err.Error(),
			Hint:   "ensure the directory exists and is readable",
			Source: spec.SourceSpan{Filename: dir},
		}
	}

	sort.Strings(paths)

	h := sha256.New()
	for _, rel := range paths {
		slashPath := filepath.ToSlash(rel)
		data, readErr := os.ReadFile(filepath.Join(dir, rel))
		if readErr != nil {
			return "", &SumError{
				Detail: "failed to read file " + slashPath + ": " + readErr.Error(),
				Hint:   "ensure all files in the directory are readable",
				Source: spec.SourceSpan{Filename: filepath.Join(dir, rel)},
			}
		}
		//nolint:errcheck,revive // sha256.Hash.Write never returns an error
		h.Write([]byte(slashPath))
		//nolint:errcheck,revive
		h.Write([]byte{0})
		//nolint:errcheck,revive
		h.Write(data)
	}

	return "h1:" + hex.EncodeToString(h.Sum(nil)), nil
}

// ReadSum parses a scampi.sum file via the source interface.
// Format per line: "<module> <version> <hash>".
// Key in the returned map is "<module> <version>".
// A missing file returns an empty map, not an error.
func ReadSum(ctx context.Context, src source.Source, path string) (map[string]string, error) {
	data, err := src.ReadFile(ctx, path)
	if err != nil {
		// Treat missing file as empty — no sums yet
		meta, statErr := src.Stat(ctx, path)
		if statErr == nil && !meta.Exists {
			return map[string]string{}, nil
		}
		return nil, &SumError{
			Detail: "failed to read scampi.sum: " + err.Error(),
			Hint:   "ensure the file is readable",
			Source: spec.SourceSpan{Filename: path},
		}
	}

	sums := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		key := fields[0] + " " + fields[1]
		sums[key] = fields[2]
	}
	return sums, nil
}

// WriteSum writes sums to a scampi.sum file via the source interface.
// Lines are sorted alphabetically by key.
func WriteSum(ctx context.Context, src source.Source, path string, sums map[string]string) error {
	keys := make([]string, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte(' ')
		sb.WriteString(sums[k])
		sb.WriteByte('\n')
	}

	if err := src.WriteFile(ctx, path, []byte(sb.String())); err != nil {
		return &SumError{
			Detail: "failed to write scampi.sum: " + err.Error(),
			Hint:   "ensure the directory is writable",
			Source: spec.SourceSpan{Filename: path},
		}
	}
	return nil
}

// VerifyModule checks whether dep's hash in modDir matches what's recorded in sums.
// If dep is not yet in sums, nil is returned (new module, not yet recorded).
//
// m is the project's parsed scampi.mod; if dep appears in m.Require, the
// resulting SumMismatchError carries the require entry's source span.
// Pass nil when no owning module is in scope.
func VerifyModule(m *Module, dep Dependency, modDir string, sums map[string]string) error {
	key := dep.Path + " " + dep.Version
	expected, ok := sums[key]
	if !ok {
		return nil
	}

	actual, err := ComputeHash(modDir)
	if err != nil {
		return err
	}

	if actual != expected {
		return &SumMismatchError{
			ModPath:  dep.Path,
			Version:  dep.Version,
			Expected: expected,
			Actual:   actual,
			Source:   depSpan(m, &dep),
		}
	}
	return nil
}
