// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseScampiImport(t *testing.T) {
	html := `<html><head>
<meta name="scampi-import" content="scampi.dev/scampi git https://codeberg.org/scampi-dev/scampi.git">
<meta name="scampi-import" content="scampi.dev/modules git https://codeberg.org/scampi-dev/modules.git">
</head></html>`

	cases := []struct {
		name       string
		importPath string
		wantPrefix string
		wantURL    string
		wantOK     bool
	}{
		{
			name:       "exact match",
			importPath: "scampi.dev/modules",
			wantPrefix: "scampi.dev/modules",
			wantURL:    "https://codeberg.org/scampi-dev/modules.git",
			wantOK:     true,
		},
		{
			name:       "subpath match",
			importPath: "scampi.dev/modules/npm",
			wantPrefix: "scampi.dev/modules",
			wantURL:    "https://codeberg.org/scampi-dev/modules.git",
			wantOK:     true,
		},
		{
			name:       "main repo",
			importPath: "scampi.dev/scampi",
			wantPrefix: "scampi.dev/scampi",
			wantURL:    "https://codeberg.org/scampi-dev/scampi.git",
			wantOK:     true,
		},
		{
			name:       "no match",
			importPath: "example.com/unknown",
			wantOK:     false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			prefix, url, ok := parseScampiImport(html, tt.importPath)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if prefix != tt.wantPrefix {
				t.Errorf("prefix = %q, want %q", prefix, tt.wantPrefix)
			}
			if url != tt.wantURL {
				t.Errorf("url = %q, want %q", url, tt.wantURL)
			}
		})
	}
}

func TestParseScampiImport_MalformedContent(t *testing.T) {
	cases := []struct {
		name string
		html string
	}{
		{"no content attr", `<meta name="scampi-import">`},
		{"empty content", `<meta name="scampi-import" content="">`},
		{"two fields only", `<meta name="scampi-import" content="scampi.dev/x git">`},
		{"wrong vcs", `<meta name="scampi-import" content="scampi.dev/x svn http://x.com">`},
		{"unclosed quote", `<meta name="scampi-import" content="scampi.dev/x git http://x.com`},
		{"empty html", "<html></html>"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok := parseScampiImport(tt.html, "scampi.dev/x")
			if ok {
				t.Error("expected no match for malformed content")
			}
		})
	}
}

func TestResolveImportPath_LiveVanity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live vanity test in short mode")
	}

	repoURL, subdir, err := resolveImportPath("scampi.dev/modules/npm")
	if err != nil {
		t.Fatalf("resolveImportPath: %v", err)
	}
	if repoURL != "https://codeberg.org/scampi-dev/modules.git" {
		t.Errorf("repoURL = %q", repoURL)
	}
	if subdir != "npm" {
		t.Errorf("subdir = %q, want 'npm'", subdir)
	}
}

func TestEnsureRemoteDep_AlreadyCached(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cached")
	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := ensureRemoteDep("example.com/nonexistent", "v0.0.0", cacheDir)
	if err != nil {
		t.Errorf("expected no-op for cached dep, got: %v", err)
	}
}

func TestEnsureRemoteDep_LiveClone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live clone test in short mode")
	}
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "npm@main")

	err := ensureRemoteDep("scampi.dev/modules/npm", "main", cacheDir)
	if err != nil {
		t.Fatalf("ensureRemoteDep: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cacheDir, "_index.scampi")); err != nil {
		t.Errorf("_index.scampi not found in cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "api.scampi")); err != nil {
		t.Errorf("api.scampi not found in cache: %v", err)
	}

	// Second call: cache hit, no-op.
	err = ensureRemoteDep("scampi.dev/modules/npm", "main", cacheDir)
	if err != nil {
		t.Errorf("second call should be no-op, got: %v", err)
	}
}
