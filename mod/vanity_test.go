// SPDX-License-Identifier: GPL-3.0-only

package mod

import "testing"

func TestParseScampiImportMeta(t *testing.T) {
	tests := []struct {
		name       string
		html       string
		modPath    string
		wantURL    string
		wantSubdir string
		wantOK     bool
	}{
		{
			name: "exact match",
			html: `<html><head>` +
				`<meta name="scampi-import" ` +
				`content="scampi.dev/modules/npm git https://github.com/scampi-dev/npm.git">` +
				`</head></html>`,
			modPath: "scampi.dev/modules/npm",
			wantURL: "https://github.com/scampi-dev/npm.git",
			wantOK:  true,
		},
		{
			name: "prefix match — subdir extracted",
			html: `<html><head>` +
				`<meta name="scampi-import" ` +
				`content="scampi.dev/modules git https://github.com/scampi-dev/modules.git">` +
				`</head></html>`,
			modPath:    "scampi.dev/modules/npm",
			wantURL:    "https://github.com/scampi-dev/modules.git",
			wantSubdir: "npm",
			wantOK:     true,
		},
		{
			name: "deep subdir",
			html: `<html><head>` +
				`<meta name="scampi-import" ` +
				`content="scampi.dev/modules git https://github.com/scampi-dev/modules.git">` +
				`</head></html>`,
			modPath:    "scampi.dev/modules/network/firewall",
			wantURL:    "https://github.com/scampi-dev/modules.git",
			wantSubdir: "network/firewall",
			wantOK:     true,
		},
		{
			name:    "no meta tag",
			html:    `<html><head><title>Hello</title></head></html>`,
			modPath: "scampi.dev/modules/npm",
			wantOK:  false,
		},
		{
			name: "wrong vcs type",
			html: `<html><head>` +
				`<meta name="scampi-import" content="scampi.dev/modules/npm svn https://example.com/repo">` +
				`</head></html>`,
			modPath: "scampi.dev/modules/npm",
			wantOK:  false,
		},
		{
			name: "prefix mismatch",
			html: `<html><head>` +
				`<meta name="scampi-import" content="other.dev/modules git https://example.com/repo.git">` +
				`</head></html>`,
			modPath: "scampi.dev/modules/npm",
			wantOK:  false,
		},
		{
			name: "malformed content — too few fields",
			html: `<html><head>` +
				`<meta name="scampi-import" content="scampi.dev/modules/npm git">` +
				`</head></html>`,
			modPath: "scampi.dev/modules/npm",
			wantOK:  false,
		},
		{
			name: "multiple meta tags — first valid wins",
			html: `<html><head>` +
				`<meta name="scampi-import" content="other.dev git https://wrong.com/repo.git">` +
				`<meta name="scampi-import" content="scampi.dev/modules/npm git https://correct.com/repo.git">` +
				`</head></html>`,
			modPath: "scampi.dev/modules/npm",
			wantURL: "https://correct.com/repo.git",
			wantOK:  true,
		},
		{
			name:    "empty html",
			html:    "",
			modPath: "scampi.dev/modules/npm",
			wantOK:  false,
		},
		{
			name: "partial prefix must be at segment boundary",
			html: `<html><head>` +
				`<meta name="scampi-import" content="scampi.dev/mod git https://example.com/repo.git">` +
				`</head></html>`,
			modPath: "scampi.dev/modules/npm",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOK := parseScampiImportMeta(tt.html, tt.modPath)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if !gotOK {
				return
			}
			if got.URL != tt.wantURL {
				t.Errorf("url = %q, want %q", got.URL, tt.wantURL)
			}
			if got.Subdir != tt.wantSubdir {
				t.Errorf("subdir = %q, want %q", got.Subdir, tt.wantSubdir)
			}
		})
	}
}

func TestExtractAttr(t *testing.T) {
	tag := `<meta name="scampi-import" content="foo bar baz">`
	if got := extractAttr(tag, "content"); got != "foo bar baz" {
		t.Errorf("content = %q, want %q", got, "foo bar baz")
	}
	if got := extractAttr(tag, "name"); got != "scampi-import" {
		t.Errorf("name = %q, want %q", got, "scampi-import")
	}
	if got := extractAttr(tag, "missing"); got != "" {
		t.Errorf("missing = %q, want empty", got)
	}
}

func TestResolveModule_AbsolutePath(t *testing.T) {
	got := resolveModule("/tmp/test/repo")
	if got.URL != "/tmp/test/repo" {
		t.Errorf("URL = %q, want /tmp/test/repo", got.URL)
	}
	if got.Subdir != "" {
		t.Errorf("Subdir = %q, want empty", got.Subdir)
	}
}
