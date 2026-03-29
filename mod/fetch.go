// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Fetch clones dep into <cacheDir>/<dep.Path>@<dep.Version>/.
// If the destination already exists, Fetch is a no-op.
// On success the .git directory is removed.
func Fetch(dep Dependency, cacheDir string) error {
	dest := filepath.Join(cacheDir, dep.Path+"@"+dep.Version)

	if _, err := os.Stat(dest); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return &FetchError{
			ModPath: dep.Path,
			Version: dep.Version,
			Detail:  fmt.Sprintf("could not create cache directory: %v", err),
			Hint:    "check that " + cacheDir + " is writable",
		}
	}

	url := gitURL(dep.Path)
	//nolint:gosec // args are derived from the parsed module manifest, not user input
	cmd := exec.Command(
		"git",
		"clone",
		"--depth=1",
		"--branch",
		dep.Version,
		"--single-branch",
		url,
		dest,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dest)
		return &FetchError{
			ModPath: dep.Path,
			Version: dep.Version,
			Detail:  firstLine(out),
			Hint:    "check that version " + dep.Version + " exists in " + url,
		}
	}

	if err := os.RemoveAll(filepath.Join(dest, ".git")); err != nil {
		_ = os.RemoveAll(dest)
		return &FetchError{
			ModPath: dep.Path,
			Version: dep.Version,
			Detail:  fmt.Sprintf("could not remove .git directory: %v", err),
			Hint:    "check permissions on " + dest,
		}
	}

	return nil
}

// gitURL returns the clone URL for a module path.
// Absolute paths (used in tests) are returned as-is.
func gitURL(modPath string) string {
	if filepath.IsAbs(modPath) {
		return modPath
	}
	return "https://" + modPath + ".git"
}

// firstLine returns the first non-empty line of b, trimmed of whitespace.
func firstLine(b []byte) string {
	first, rest, found := bytes.Cut(b, []byte{'\n'})
	line := string(bytes.TrimSpace(first))
	if line != "" || !found {
		return line
	}
	return string(bytes.TrimSpace(rest))
}
