// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"path/filepath"
	"strings"

	"scampi.dev/scampi/source"
)

// Resolve maps a module load path to an absolute .scampi file path.
//
// For bare module loads (e.g. codeberg.org/user/repo), the entry point is
// resolved by trying _index.scampi then <last-segment>.scampi. If both
// exist, _index.scampi takes precedence.
//
// For subpath loads (e.g. codeberg.org/user/repo/internal/helpers), the
// subpath is resolved by trying <subpath>.scampi then <subpath>/_index.scampi.
//
// Remote deps resolve against cacheDir. Local deps resolve against the
// filesystem path in the version field (relative to scampi.mod's directory).
func Resolve(ctx context.Context, src source.Source, m *Module, loadPath string, cacheDir string) (string, error) {
	dep, subPath := splitModulePath(m, loadPath)
	if dep == nil {
		return "", &ModuleNotFoundError{LoadPath: loadPath}
	}

	modDir := modDirFor(m, dep, cacheDir)
	meta, err := src.Stat(ctx, modDir)
	if err != nil {
		return "", err
	}
	if !meta.Exists {
		return "", &ModuleNotCachedError{
			ModPath: dep.Path,
			Version: dep.Version,
			Source:  m.DepSpan(dep),
		}
	}

	candidates := resolveCandidates(modDir, dep, subPath)

	for _, candidate := range candidates {
		cmeta, err := src.Stat(ctx, candidate)
		if err != nil {
			return "", err
		}
		if cmeta.Exists && !cmeta.IsDir {
			return candidate, nil
		}
	}

	tried := make([]string, len(candidates))
	for i, c := range candidates {
		tried[i] = strings.TrimPrefix(c, modDir+string(filepath.Separator))
	}
	return "", &ModuleNoEntryPointError{
		ModPath: dep.Path,
		Tried:   tried,
	}
}

// ValidateEntryPoint checks that a module directory contains a loadable
// .scampi entry point. Returns NotAModuleError if not.
func ValidateEntryPoint(ctx context.Context, src source.Source, dep Dependency, dir string) error {
	if hasEntryPoint(ctx, src, dir, lastSegment(dep.Path)) {
		return nil
	}
	return &NotAModuleError{ModPath: dep.Path, Version: dep.Version}
}

// modDirFor returns the directory where a dependency's files live.
func modDirFor(m *Module, dep *Dependency, cacheDir string) string {
	if dep.IsLocal() {
		dir := dep.Version
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(m.Filename), dir)
		}
		return dir
	}
	return filepath.Join(cacheDir, dep.Path+"@"+dep.Version)
}

// resolveCandidates returns the ordered list of candidate .scampi paths to try.
func resolveCandidates(modDir string, dep *Dependency, subPath string) []string {
	if subPath == "" {
		last := lastSegment(dep.Path)
		return []string{
			filepath.Join(modDir, "_index.scampi"),
			filepath.Join(modDir, last+".scampi"),
		}
	}

	subNative := filepath.FromSlash(subPath)
	return []string{
		filepath.Join(modDir, subNative+".scampi"),
		filepath.Join(modDir, subNative, "_index.scampi"),
	}
}

// splitModulePath finds the longest require-table prefix of loadPath and
// returns the matching Dependency and any remaining subpath.
func splitModulePath(m *Module, loadPath string) (*Dependency, string) {
	var best *Dependency
	for i := range m.Require {
		dep := &m.Require[i]
		if loadPath == dep.Path {
			return dep, ""
		}
		if strings.HasPrefix(loadPath, dep.Path+"/") {
			if best == nil || len(dep.Path) > len(best.Path) {
				best = dep
			}
		}
	}
	if best != nil {
		sub := strings.TrimPrefix(loadPath, best.Path+"/")
		return best, sub
	}
	return nil, ""
}

// lastSegment returns the last path segment of a slash-separated path.
func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func hasEntryPoint(ctx context.Context, src source.Source, dir, name string) bool {
	if meta, err := src.Stat(ctx, filepath.Join(dir, "_index.scampi")); err == nil && meta.Exists {
		return true
	}
	if meta, err := src.Stat(ctx, filepath.Join(dir, name+".scampi")); err == nil && meta.Exists {
		return true
	}
	return false
}

// DefaultCacheDir returns the default module cache directory.
// Uses $XDG_CACHE_HOME/scampi/mod if set, else ~/.cache/scampi/mod.
func DefaultCacheDir() string {
	return defaultCacheDir()
}
