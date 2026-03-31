// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"scampi.dev/scampi/source"
)

// Add adds or updates a dependency in scampi.mod (and scampi.sum for remote deps).
// For remote modules, version may be empty to resolve the latest stable tag.
// For local modules, version is a filesystem path (starts with . or /).
// Returns the resolved version and what changed in scampi.mod.
func Add(
	ctx context.Context,
	src source.Source,
	modPath string,
	version string,
	dir string,
	cacheDir string,
) (string, ModFileChange, error) {
	dep := Dependency{Path: modPath, Version: version}

	if dep.IsLocal() {
		return addLocal(ctx, src, dep, dir)
	}
	return addRemote(ctx, src, dep, dir, cacheDir)
}

func addLocal(ctx context.Context, src source.Source, dep Dependency, dir string) (string, ModFileChange, error) {
	localDir := dep.Version
	if !filepath.IsAbs(localDir) {
		localDir = filepath.Join(dir, localDir)
	}

	if err := ValidateEntryPoint(ctx, src, dep, localDir); err != nil {
		return "", 0, err
	}

	change, err := updateModFile(ctx, src, dep, dir)
	return dep.Version, change, err
}

func addRemote(
	ctx context.Context,
	src source.Source,
	dep Dependency,
	dir string,
	cacheDir string,
) (string, ModFileChange, error) {
	if dep.Version == "" {
		resolved, err := resolveLatestStable(dep.Path)
		if err != nil {
			return "", 0, err
		}
		dep.Version = resolved
	}

	if err := Fetch(dep, cacheDir); err != nil {
		return "", 0, err
	}

	dest := filepath.Join(cacheDir, dep.Path+"@"+dep.Version)

	if err := ValidateEntryPoint(ctx, src, dep, dest); err != nil {
		_ = os.RemoveAll(dest)
		return "", 0, err
	}

	hash, err := ComputeHash(dest)
	if err != nil {
		return "", 0, err
	}

	// Read existing mod file and compute the change type.
	modFile := filepath.Join(dir, "scampi.mod")
	data, err := src.ReadFile(ctx, modFile)
	if err != nil {
		return "", 0, &AddError{
			Detail: "could not read scampi.mod: " + err.Error(),
			Hint:   "run: scampi mod init",
		}
	}
	m, err := Parse(modFile, data)
	if err != nil {
		return "", 0, err
	}

	deps := make([]Dependency, 0, len(m.Require)+1)
	change := ModFileAdded
	for _, d := range m.Require {
		if d.Path == dep.Path {
			if d.Version == dep.Version {
				change = ModFileUnchanged
			} else {
				change = ModFileUpdated
			}
			deps = append(deps, Dependency{Path: dep.Path, Version: dep.Version})
		} else {
			deps = append(deps, d)
		}
	}
	if change == ModFileAdded {
		deps = append(deps, Dependency{Path: dep.Path, Version: dep.Version})
	}

	// Resolve and fetch transitive dependencies.
	allDeps, err := FetchTransitive(ctx, src, deps, cacheDir)
	if err != nil {
		return "", 0, err
	}

	sumFile := filepath.Join(dir, "scampi.sum")
	sums, err := ReadSum(ctx, src, sumFile)
	if err != nil {
		return "", 0, err
	}

	key := dep.Path + " " + dep.Version
	sums[key] = hash

	for _, tdep := range allDeps {
		if !tdep.Indirect || tdep.IsLocal() {
			continue
		}
		tdest := filepath.Join(cacheDir, tdep.Path+"@"+tdep.Version)
		if err := ValidateEntryPoint(ctx, src, tdep, tdest); err != nil {
			return "", 0, err
		}
		h, err := ComputeHash(tdest)
		if err != nil {
			return "", 0, err
		}
		sums[tdep.Path+" "+tdep.Version] = h
	}

	if err := WriteSum(ctx, src, sumFile, sums); err != nil {
		return "", 0, err
	}

	slices.SortFunc(allDeps, func(a, b Dependency) int {
		return strings.Compare(a.Path, b.Path)
	})

	if err := WriteModFile(ctx, src, modFile, m.Module, allDeps); err != nil {
		return "", 0, err
	}

	return dep.Version, change, nil
}

// ModFileChange describes what updateModFile did.
type ModFileChange int

const (
	ModFileAdded     ModFileChange = iota // new entry added
	ModFileUpdated                        // existing entry version changed
	ModFileUnchanged                      // already present with same version
)

func updateModFile(ctx context.Context, src source.Source, dep Dependency, dir string) (ModFileChange, error) {
	modFile := filepath.Join(dir, "scampi.mod")
	data, err := src.ReadFile(ctx, modFile)
	if err != nil {
		return 0, &AddError{
			Detail: "could not read scampi.mod: " + err.Error(),
			Hint:   "run: scampi mod init",
		}
	}

	m, err := Parse(modFile, data)
	if err != nil {
		return 0, err
	}

	deps := make([]Dependency, 0, len(m.Require)+1)
	change := ModFileAdded
	for _, d := range m.Require {
		if d.Path == dep.Path {
			if d.Version == dep.Version {
				change = ModFileUnchanged
			} else {
				change = ModFileUpdated
			}
			deps = append(deps, Dependency{Path: dep.Path, Version: dep.Version})
		} else {
			deps = append(deps, Dependency{Path: d.Path, Version: d.Version})
		}
	}
	if change == ModFileAdded {
		deps = append(deps, Dependency{Path: dep.Path, Version: dep.Version})
	}

	if change == ModFileUnchanged {
		return ModFileUnchanged, nil
	}

	slices.SortFunc(deps, func(a, b Dependency) int {
		return strings.Compare(a.Path, b.Path)
	})

	return change, WriteModFile(ctx, src, modFile, m.Module, deps)
}

// resolveLatestStable runs git ls-remote --tags on the module URL and returns
// the highest stable semver tag. Returns NoStableVersionError if none found.
func resolveLatestStable(modPath string) (string, error) {
	url := gitURL(modPath)
	//nolint:gosec // modPath is from the parsed module manifest, not raw user input
	cmd := exec.Command(
		"git",
		"ls-remote",
		"--tags",
		url,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", &AddError{
			Detail: "could not list tags for " + modPath + ": " + firstLine(out),
			Hint:   "check that " + url + " is accessible",
		}
	}

	version := ParseLatestStable(string(out))
	if version == "" {
		return "", &NoStableVersionError{ModPath: modPath}
	}
	return version, nil
}

// ParseLatestStable parses git ls-remote --tags output and returns the
// highest stable semver tag. Returns "" if no stable tags are found.
func ParseLatestStable(output string) string {
	var stable []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "<hash>\trefs/tags/<tag>"
		_, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		// Skip dereferenced tags
		if strings.HasSuffix(ref, "^{}") {
			continue
		}
		tag, ok := strings.CutPrefix(ref, "refs/tags/")
		if !ok {
			continue
		}
		if !isSemver(tag) {
			continue
		}
		// Stable = no pre-release suffix
		rest := tag[1:] // strip 'v'
		if strings.ContainsRune(rest, '-') {
			continue
		}
		stable = append(stable, tag)
	}

	if len(stable) == 0 {
		return ""
	}

	slices.SortFunc(stable, compareSemver)
	return stable[len(stable)-1]
}

// compareSemver compares two semver strings for use with slices.SortFunc.
// Returns negative if a < b, zero if equal, positive if a > b.
func compareSemver(a, b string) int {
	pa := parseSemverParts(a)
	pb := parseSemverParts(b)
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

// parseSemverParts extracts [major, minor, patch] from a semver string like "v1.2.3".
func parseSemverParts(v string) [3]int {
	rest := strings.TrimPrefix(v, "v")
	// Strip pre-release suffix
	if idx := strings.IndexByte(rest, '-'); idx >= 0 {
		rest = rest[:idx]
	}
	parts := strings.SplitN(rest, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}
