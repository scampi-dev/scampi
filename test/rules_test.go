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
		"os/*",
		"runtime",
		"runtime/*",
		"syscall",
		"syscall/*",
		"net",
		"net/*",
		"crypto",
		"crypto/*",

		"github.com/pkg/sftp",
		"github.com/pkg/sftp/*",
		"golang.org/x/crypto/ssh",
		"golang.org/x/crypto/ssh/*",
	}

	allowAll := func() string {
		return strings.Join(restrictedImports, ",")
	}

	// ---- capability rules (human-readable policy) ----
	rules := []capabilityRule{
		{
			pattern:        "bin/**/*",
			allowedImports: allowAll(),
		},
		{
			pattern:        "cmd/main.go",
			allowedImports: "os,os/signal,runtime/debug",
		},
		{
			pattern:        "osutil/diff.go",
			allowedImports: "os,os/exec",
		},
		{
			pattern:        "engine/errors.go",
			allowedImports: "runtime",
		},
		{
			pattern:        "render/cli/cli.go",
			allowedImports: "os",
		},
		{
			pattern:        "source/local_posix.go",
			allowedImports: "os",
		},
		{
			pattern:        "target/local/posix.go",
			allowedImports: "os,os/exec,os/user,syscall",
		},
		{
			pattern:        "target/ssh/errors.go",
			allowedImports: "golang.org/x/crypto/ssh/knownhosts",
		},
		{
			pattern: "target/ssh/ssh.go",
			allowedImports: `net,os,
			golang.org/x/crypto/ssh,
			golang.org/x/crypto/ssh/agent,
			golang.org/x/crypto/ssh/knownhosts,
			github.com/pkg/sftp`,
		},
		{
			pattern: "target/ssh/target.go",
			allowedImports: `os,
			golang.org/x/crypto/ssh,
			github.com/pkg/sftp`,
		},
		{
			pattern:        "osutil/signals_unix.go",
			allowedImports: "os,syscall",
		},
		{
			pattern:        "osutil/signals_windows.go",
			allowedImports: "os",
		},
		{
			pattern:        "test/harness.go",
			allowedImports: "os",
		},
		{
			pattern:        "test/ssh_harness.go",
			allowedImports: "os,os/exec,net",
		},
		{
			pattern:        "test/ssh_connection_test.go",
			allowedImports: "os",
		},
		{
			pattern:        "test/e2e_driver_test.go",
			allowedImports: "os",
		},
		{
			pattern:        "test/main_test.go",
			allowedImports: "os",
		},
		{
			pattern:        "cmd/usage_test.go",
			allowedImports: "os,os/exec",
		},
		{
			pattern:        "cmd/fuzz_test.go",
			allowedImports: "os/exec",
		},
	}

	splitList := func(s string) []string {
		parts := strings.Split(s, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}

	// matchImport checks if an import path matches a pattern.
	// Patterns ending in /* match any sub-import but not the base.
	// e.g., "os/*" matches "os/exec" but not "os"
	matchImport := func(pattern, importPath string) bool {
		if strings.HasSuffix(pattern, "/*") {
			base := strings.TrimSuffix(pattern, "/*")
			return strings.HasPrefix(importPath, base+"/")
		}
		return pattern == importPath
	}

	// isRestricted checks if an import matches any restricted pattern
	isRestricted := func(importPath string) bool {
		for _, r := range restrictedImports {
			if matchImport(r, importPath) {
				return true
			}
		}
		return false
	}

	// isAllowed checks if an import is allowed by the given allowed list
	isAllowed := func(importPath string, allowed []string) bool {
		for _, a := range allowed {
			if matchImport(a, importPath) {
				return true
			}
		}
		return false
	}

	// Track which allowed imports are actually used per rule (by index)
	usedImports := make([]map[string]bool, len(rules))
	for i := range rules {
		usedImports[i] = make(map[string]bool)
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

		// compute allowed imports for this file and track matching rule indices
		var allowed []string
		var matchingRules []int
		for i, r := range rules {
			if match, _ := path.Match(r.pattern, rel); match {
				allowed = append(allowed, splitList(r.allowedImports)...)
				matchingRules = append(matchingRules, i)
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
			if isRestricted(pathVal) {
				if !isAllowed(pathVal, allowed) {
					t.Errorf(
						`illegal import %q in %s (not allowed by capability rules)`,
						pathVal,
						rel,
					)
				} else {
					// Mark this import as used for all matching rules
					for _, ruleIdx := range matchingRules {
						for _, allowedPattern := range splitList(rules[ruleIdx].allowedImports) {
							if matchImport(allowedPattern, pathVal) {
								usedImports[ruleIdx][allowedPattern] = true
							}
						}
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// ---- check for unused allowed imports (excludes allowAll rules) ----
	for i, r := range rules {
		if r.allowedImports == allowAll() {
			continue // skip rules that allow everything
		}
		for _, imp := range splitList(r.allowedImports) {
			if !usedImports[i][imp] {
				t.Errorf(
					`unused allowed import %q in rule for %q (remove from allowedImports)`,
					imp,
					r.pattern,
				)
			}
		}
	}
}
