package test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type capabilityRule struct {
	pattern        string // POSIX-style path, e.g. source/local_*.go
	allowedImports string // comma-delimited list
}

func TestImportCapabilities(t *testing.T) {
	root := ".."

	// ---- hard global bans (no exceptions) ----
	globallyForbidden := []string{
		"unsafe",
	}

	// ---- restricted imports (require explicit capability) ----
	restrictedImports := []string{
		"os",
		"runtime",
		"syscall",
		"net",
		"net/http",
		"net/url",
		"crypto/tls",
	}

	allowAll := func() string {
		return strings.Join(restrictedImports, ",")
	}
	_ = allowAll

	// ---- capability rules (human-readable policy) ----
	rules := []capabilityRule{
		{
			pattern:        "bin/**/*",
			allowedImports: allowAll(),
		},
		{
			pattern:        "cmd/main.go",
			allowedImports: "os",
		},
		{
			pattern:        "engine/errors.go",
			allowedImports: "runtime",
		},
		{
			pattern:        "render/cli.go",
			allowedImports: "os",
		},
		{
			pattern:        "source/local_*.go",
			allowedImports: "os",
		},
		{
			pattern:        "target/local/posix.go",
			allowedImports: "os,syscall",
		},
		{
			pattern:        "osutil/*.go",
			allowedImports: "os,syscall",
		},
		{
			pattern:        "test/harness.go",
			allowedImports: "os",
		},
	}

	splitList := func(s string) []string {
		parts := strings.Split(s, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}

		// normalize to POSIX-style relative path
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		// compute allowed imports for this file
		var allowed []string
		for _, r := range rules {
			if match, _ := path.Match(r.pattern, rel); match {
				allowed = append(allowed, splitList(r.allowedImports)...)
			}
		}

		for _, imp := range file.Imports {
			pathVal, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				panic(err)
			}

			// ---- global hard ban ----
			if slices.Contains(globallyForbidden, pathVal) {
				t.Errorf(
					`illegal import %q in %s (forbidden globally)`,
					pathVal,
					rel,
				)
			}

			// ---- restricted imports need explicit permission ----
			if slices.Contains(restrictedImports, pathVal) &&
				!slices.Contains(allowed, pathVal) {

				t.Errorf(
					`illegal import %q in %s (not allowed by capability rules)`,
					pathVal,
					rel,
				)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
