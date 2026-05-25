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
