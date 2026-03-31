// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"scampi.dev/scampi/source"
)

// ResolveDeps performs minimum version selection on the given direct
// dependencies and their transitive requirements. It returns a flat list
// containing direct deps (Indirect=false) followed by transitive deps
// (Indirect=true), sorted by path. Local deps are passed through as-is
// without resolving their transitive requirements.
func ResolveDeps(
	ctx context.Context,
	src source.Source,
	direct []Dependency,
	cacheDir string,
) ([]Dependency, error) {
	// selected tracks the highest version seen for each module path.
	selected := make(map[string]string, len(direct))

	// directPaths remembers which paths came from the direct list so we
	// can mark everything else as indirect.
	directPaths := make(map[string]bool, len(direct))

	for _, d := range direct {
		if !d.Indirect {
			directPaths[d.Path] = true
		}
		if d.IsLocal() {
			continue
		}
		selected[d.Path] = d.Version
	}

	// BFS over transitive deps.
	queue := make([]Dependency, 0, len(direct))
	for _, d := range direct {
		if !d.IsLocal() {
			queue = append(queue, d)
		}
	}

	// visited tracks which path@version pairs we've already read, to
	// avoid re-reading the same scampi.mod.
	visited := make(map[string]bool)

	for len(queue) > 0 {
		dep := queue[0]
		queue = queue[1:]

		key := dep.Path + "@" + dep.Version
		if visited[key] {
			continue
		}
		visited[key] = true

		m, err := readModFromCache(ctx, src, dep, cacheDir)
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}

		for _, req := range m.Require {
			if req.IsLocal() {
				continue
			}
			prev, ok := selected[req.Path]
			if !ok || compareSemver(req.Version, prev) > 0 {
				selected[req.Path] = req.Version
			}
			queue = append(queue, Dependency{Path: req.Path, Version: req.Version})
		}
	}

	// Cycle detection via DFS on the resolved graph.
	if err := detectCycles(ctx, src, selected, cacheDir); err != nil {
		return nil, err
	}

	// Build result list.
	result := make([]Dependency, 0, len(selected)+countLocal(direct))

	for _, d := range direct {
		if d.IsLocal() {
			result = append(result, Dependency{
				Path:     d.Path,
				Version:  d.Version,
				Indirect: false,
			})
		}
	}

	for path, version := range selected {
		result = append(result, Dependency{
			Path:     path,
			Version:  version,
			Indirect: !directPaths[path],
		})
	}

	slices.SortFunc(result, func(a, b Dependency) int {
		return strings.Compare(a.Path, b.Path)
	})

	return result, nil
}

// readModFromCache reads a module's scampi.mod from the cache directory.
// Returns nil (not an error) if the file doesn't exist — the module simply
// has no dependencies.
func readModFromCache(
	ctx context.Context,
	src source.Source,
	dep Dependency,
	cacheDir string,
) (*Module, error) {
	modFile := filepath.Join(cacheDir, dep.Path+"@"+dep.Version, "scampi.mod")

	meta, err := src.Stat(ctx, modFile)
	if err != nil {
		return nil, err
	}
	if !meta.Exists {
		return nil, nil
	}

	data, err := src.ReadFile(ctx, modFile)
	if err != nil {
		return nil, err
	}
	return Parse(modFile, data)
}

// Cycle detection
// -----------------------------------------------------------------------------

func detectCycles(
	ctx context.Context,
	src source.Source,
	selected map[string]string,
	cacheDir string,
) error {
	const (
		white = 0 // unvisited
		grey  = 1 // in current DFS path
		black = 2 // fully explored
	)

	color := make(map[string]int, len(selected))
	parent := make(map[string]string, len(selected))

	var dfs func(path string) error
	dfs = func(path string) error {
		color[path] = grey

		version, ok := selected[path]
		if !ok {
			color[path] = black
			return nil
		}

		dep := Dependency{Path: path, Version: version}
		m, err := readModFromCache(ctx, src, dep, cacheDir)
		if err != nil {
			return err
		}
		if m != nil {
			for _, req := range m.Require {
				if req.IsLocal() {
					continue
				}
				switch color[req.Path] {
				case grey:
					chain := buildCycleChain(parent, path, req.Path)
					return &CycleError{Chain: chain}
				case white:
					parent[req.Path] = path
					if err := dfs(req.Path); err != nil {
						return err
					}
				}
			}
		}

		color[path] = black
		return nil
	}

	for path := range selected {
		if color[path] == white {
			if err := dfs(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildCycleChain(parent map[string]string, from, to string) []string {
	var chain []string
	cur := from
	for cur != to {
		chain = append(chain, cur)
		cur = parent[cur]
	}
	chain = append(chain, to)

	// Reverse so the chain reads start → ... → end → start.
	slices.Reverse(chain)
	chain = append(chain, to)
	return chain
}

func countLocal(deps []Dependency) int {
	n := 0
	for _, d := range deps {
		if d.IsLocal() {
			n++
		}
	}
	return n
}

// FetchTransitive resolves transitive dependencies and fetches any that
// aren't already cached. It iterates until the graph is fully resolved
// (deeper transitive deps may only be discoverable after their parents
// are fetched). Returns the complete dependency list.
func FetchTransitive(
	ctx context.Context,
	src source.Source,
	direct []Dependency,
	cacheDir string,
) ([]Dependency, error) {
	for {
		allDeps, err := ResolveDeps(ctx, src, direct, cacheDir)
		if err != nil {
			return nil, err
		}

		fetched := 0
		for _, dep := range allDeps {
			if dep.IsLocal() {
				continue
			}
			dest := filepath.Join(cacheDir, dep.Path+"@"+dep.Version)
			if _, err := os.Stat(dest); err == nil {
				continue
			}
			if err := Fetch(dep, cacheDir); err != nil {
				return nil, err
			}
			fetched++
		}

		if fetched == 0 {
			return allDeps, nil
		}
	}
}
