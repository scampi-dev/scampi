// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"scampi.dev/scampi/errs"
)

// ensureRemoteDep checks whether a remote dependency is already
// cached. If not, it resolves the git repo URL (via vanity import
// meta tags or direct clone), clones at the specified version, and
// stores the result in the module cache.
//
// The cache layout is:
//
//	~/.cache/scampi/mod/<path>@<version>/
//
// Once cached, subsequent loads read from disk without network.
func ensureRemoteDep(depPath, version, cacheDir string) error {
	if _, err := os.Stat(cacheDir); err == nil {
		return nil // already cached
	}

	repoURL, subdir, err := resolveImportPath(depPath)
	if err != nil {
		// bare-error: module resolution infrastructure
		return errs.Errorf("resolve %s: %w", depPath, err)
	}

	// Clone into a temp dir, then move to the cache atomically.
	tmp, err := os.MkdirTemp("", "scampi-mod-*")
	if err != nil {
		// bare-error: module resolution infrastructure
		return errs.Errorf("tmpdir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// version can be a tag, branch, or commit hash. For tags and
	// branches, --branch works with --depth=1. For hashes, we need
	// a full clone then checkout — but try --branch first since
	// it covers the common case efficiently.
	cloneArgs := []string{"clone", "--depth=1"}
	if version != "" {
		cloneArgs = append(cloneArgs, "--branch", version)
	}
	cloneArgs = append(cloneArgs, repoURL, tmp)

	cmd := exec.Command("git", cloneArgs...)
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		// bare-error: module resolution infrastructure
		return errs.Errorf("git clone %s@%s: %w", repoURL, version, err)
	}

	// Source directory: the whole clone or a subdirectory for
	// monorepo modules.
	srcDir := tmp
	if subdir != "" {
		srcDir = filepath.Join(tmp, subdir)
		if _, err := os.Stat(srcDir); err != nil {
			// bare-error: module resolution infrastructure
			return errs.Errorf("subdir %s not found in %s", subdir, repoURL)
		}
	}

	// Move to cache.
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return err
	}
	return os.Rename(srcDir, cacheDir)
}

// resolveImportPath probes the vanity URL for a scampi-import meta
// tag. If found, returns the git URL and any subdirectory. If not,
// falls back to probing progressively shorter .git URLs.
func resolveImportPath(importPath string) (repoURL, subdir string, err error) {
	url := "https://" + importPath + "?scampi-get=1"
	resp, err := http.Get(url)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			if prefix, vcsURL, ok := parseScampiImport(string(body), importPath); ok {
				sub := ""
				if len(importPath) > len(prefix) {
					sub = strings.TrimPrefix(importPath[len(prefix):], "/")
				}
				return vcsURL, sub, nil
			}
		}
	}

	// Fallback: probe progressively shorter prefixes as .git URLs.
	parts := strings.Split(importPath, "/")
	for i := len(parts); i >= 2; i-- {
		candidate := "https://" + strings.Join(parts[:i], "/") + ".git"
		sub := ""
		if i < len(parts) {
			sub = strings.Join(parts[i:], "/")
		}
		cmd := exec.Command("git", "ls-remote", "--exit-code", candidate)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if cmd.Run() == nil {
			return candidate, sub, nil
		}
	}

	// bare-error: module resolution infrastructure
	return "", "", errs.Errorf("no git repository found for %s", importPath)
}

// parseScampiImport extracts the repo URL from a scampi-import meta
// tag whose prefix matches the import path.
func parseScampiImport(html, importPath string) (prefix, repoURL string, ok bool) {
	for _, line := range strings.Split(html, "<meta") {
		if !strings.Contains(line, "scampi-import") {
			continue
		}
		idx := strings.Index(line, `content="`)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(`content="`):]
		end := strings.IndexByte(rest, '"')
		if end < 0 {
			continue
		}
		fields := strings.Fields(rest[:end])
		if len(fields) != 3 || fields[1] != "git" {
			continue
		}
		if strings.HasPrefix(importPath, fields[0]) {
			return fields[0], fields[2], true
		}
	}
	return "", "", false
}
