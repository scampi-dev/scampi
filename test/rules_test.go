// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
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
			pattern:        "osutil/configdir.go",
			allowedImports: "os",
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
			pattern:        "target/local/local.go",
			allowedImports: "os",
		},
		{
			pattern:        "target/local/posix.go",
			allowedImports: "os,os/exec,os/user,syscall,crypto/rand",
		},
		{
			pattern:        "target/local/local_test.go",
			allowedImports: "runtime",
		},
		{
			pattern:        "target/local/escalate_test.go",
			allowedImports: "os",
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
			allowedImports: `os,crypto/rand,
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
			pattern:        "test/rules_test.go",
			allowedImports: "os",
		},
		{
			pattern:        "cmd/secrets.go",
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

// Function signature formatting
// -----------------------------------------------------------------------------

func TestFuncSignatureStyle(t *testing.T) {
	root := ".."

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				checkFieldList(t, fset, rel, n.Name.Name, "params", n.Type.Params)
				checkFieldList(t, fset, rel, n.Name.Name, "results", n.Type.Results)
			case *ast.FuncLit:
				checkFieldList(t, fset, rel, "(func literal)", "params", n.Type.Params)
				checkFieldList(t, fset, rel, "(func literal)", "results", n.Type.Results)
			case *ast.CallExpr:
				checkCallArgs(t, fset, rel, n)
			}
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func callName(expr ast.Expr) string {
	switch fn := expr.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return callName(fn.X) + "." + fn.Sel.Name
	}
	return "(call)"
}

func argSpansOneLine(fset *token.FileSet, arg ast.Expr) bool {
	return fset.Position(arg.Pos()).Line == fset.Position(arg.End()).Line
}

var formatFuncSuffixes = []string{
	"Sprintf", "Errorf", "Fatalf", "Logf", "Skipf",
	"fmtfMsg", "fmtfMsgTo", "BUG",
}

func isFormatCall(name string) bool {
	for _, suffix := range formatFuncSuffixes {
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return name == "fmt.Sprintf" || name == "fmt.Errorf"
}

func isExcludedCall(name string) bool {
	if isFormatCall(name) {
		return true
	}
	return strings.HasSuffix(name, ".UnpackArgs") || name == "UnpackArgs"
}

func checkCallArgs(
	t *testing.T,
	fset *token.FileSet,
	file string,
	call *ast.CallExpr,
) {
	t.Helper()
	if len(call.Args) <= 1 {
		return
	}

	name := callName(call.Fun)
	if isExcludedCall(name) {
		return
	}

	openLine := fset.Position(call.Lparen).Line
	closeLine := fset.Position(call.Rparen).Line

	if openLine == closeLine {
		return
	}

	// Skip if any argument spans multiple lines (nested calls, func
	// literals, composite literals, etc). These naturally make the
	// outer call multi-line without it being a formatting issue.
	for _, arg := range call.Args {
		if !argSpansOneLine(fset, arg) {
			return
		}
	}

	seen := map[int]bool{}
	for _, arg := range call.Args {
		line := fset.Position(arg.Pos()).Line
		if seen[line] {
			t.Errorf(
				"%s:%d: %s: multi-line call must have one argument per line",
				file, line, name,
			)
			break
		}
		seen[line] = true
	}
}

func checkFieldList(
	t *testing.T,
	fset *token.FileSet,
	file, funcName, label string,
	fl *ast.FieldList,
) {
	t.Helper()
	if fl == nil || len(fl.List) <= 1 {
		return
	}

	openLine := fset.Position(fl.Opening).Line
	closeLine := fset.Position(fl.Closing).Line

	if openLine == closeLine {
		return // all on one line — fine
	}

	// Multi-line: each field must be on its own line.
	seen := map[int]bool{}
	for _, field := range fl.List {
		line := fset.Position(field.Pos()).Line
		if seen[line] {
			t.Errorf(
				"%s:%d: %s %s: multi-line signature must have one parameter per line",
				file, line, funcName, label,
			)
			break
		}
		seen[line] = true
	}
}

// Markdown table alignment
// -----------------------------------------------------------------------------

func TestMarkdownTableAlignment(t *testing.T) {
	root := ".."

	isTableRow := func(line string) bool {
		return len(line) >= 3 && line[0] == '|' && line[len(line)-1] == '|'
	}

	isSeparatorRow := func(line string) bool {
		for _, c := range line {
			switch c {
			case '|', '-', ':', ' ':
			default:
				return false
			}
		}
		return true
	}

	checkTable := func(t *testing.T, rel string, rows []string, startLine int) {
		t.Helper()
		if len(rows) < 2 {
			return
		}
		if !isSeparatorRow(rows[1]) {
			return
		}

		wantCols := strings.Count(rows[0], "|") - 1
		wantLen := utf8.RuneCountInString(rows[0])

		for i, row := range rows {
			lineNum := startLine + i

			gotCols := strings.Count(row, "|") - 1
			if gotCols != wantCols {
				t.Errorf(
					"%s:%d: table row has %d columns, want %d (same as header at line %d)",
					rel, lineNum, gotCols, wantCols, startLine,
				)
				continue
			}

			gotLen := utf8.RuneCountInString(row)
			if gotLen != wantLen {
				t.Errorf(
					"%s:%d: table row length %d, want %d (same as header at line %d)",
					rel, lineNum, gotLen, wantLen, startLine,
				)
			}
		}
	}

	walkFile := func(t *testing.T, rel, abs string) {
		t.Helper()
		f, err := os.Open(abs)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()

		var tableRows []string
		tableStart := 0

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			if isTableRow(line) {
				if len(tableRows) == 0 {
					tableStart = lineNum
				}
				tableRows = append(tableRows, line)
			} else {
				if len(tableRows) > 0 {
					checkTable(t, rel, tableRows, tableStart)
					tableRows = tableRows[:0]
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		if len(tableRows) > 0 {
			checkTable(t, rel, tableRows, tableStart)
		}
	}

	// Walk site/content/ and docs/
	for _, dir := range []string{"site/content", "docs"} {
		abs := filepath.Join(root, dir)
		err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			walkFile(t, rel, p)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check README.md
	readme := filepath.Join(root, "README.md")
	if _, err := os.Stat(readme); err == nil {
		walkFile(t, "README.md", readme)
	}
}
