// SPDX-License-Identifier: GPL-3.0-only

package rules

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

func Test_Rule_ImportCapabilities(t *testing.T) {
	root := repoRoot(t)

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
			pattern:        "cmd/scampi/main.go",
			allowedImports: "os,os/signal,runtime/debug",
		},
		{
			pattern:        "cmd/scampi/profile.go",
			allowedImports: "os,runtime,runtime/pprof,runtime/trace",
		},
		{
			pattern:        "internal/linker/usermod.go",
			allowedImports: "os",
		},
		{
			pattern:        "internal/step/sharedop/convert.go",
			allowedImports: "crypto/sha256",
		},
		{
			pattern:        "internal/step/pkg/convert.go",
			allowedImports: "crypto/sha256,net/url",
		},
		{
			pattern:        "internal/osutil/configdir.go",
			allowedImports: "os",
		},
		{
			pattern:        "internal/osutil/diff.go",
			allowedImports: "os,os/exec",
		},
		{
			pattern:        "internal/osutil/fuzzy.go",
			allowedImports: "os,os/exec",
		},
		{
			pattern:        "internal/engine/errors.go",
			allowedImports: "runtime",
		},
		{
			pattern:        "internal/render/cli/cli.go",
			allowedImports: "os",
		},
		{
			pattern:        "internal/source/local_posix.go",
			allowedImports: "os",
		},
		{
			pattern:        "internal/target/local/local.go",
			allowedImports: "os",
		},
		{
			pattern:        "internal/target/local/posix.go",
			allowedImports: "os,os/exec,os/user",
		},
		{
			pattern:        "internal/target/local/escalate.go",
			allowedImports: "os,crypto/rand",
		},
		{
			pattern:        "internal/target/local/repo.go",
			allowedImports: "os",
		},
		{
			pattern:        "internal/target/local/stat_linux.go",
			allowedImports: "os,os/user,syscall",
		},
		{
			pattern:        "internal/target/local/stat_bsd.go",
			allowedImports: "os,os/user,syscall",
		},
		{
			pattern:        "internal/target/ssh/errors.go",
			allowedImports: "golang.org/x/crypto/ssh/knownhosts",
		},
		{
			pattern: "internal/target/ssh/ssh.go",
			allowedImports: `net,os,
			golang.org/x/crypto/ssh,
			golang.org/x/crypto/ssh/agent,
			golang.org/x/crypto/ssh/knownhosts,
			github.com/pkg/sftp`,
		},
		{
			pattern: "internal/target/ssh/target.go",
			allowedImports: `os,crypto/rand,
			golang.org/x/crypto/ssh,
			github.com/pkg/sftp`,
		},
		{
			pattern: "internal/target/ssh/shell_session.go",
			allowedImports: `crypto/rand,
			golang.org/x/crypto/ssh`,
		},
		{
			pattern:        "internal/osutil/signals_unix.go",
			allowedImports: "os,syscall",
		},
		{
			pattern:        "internal/osutil/signals_windows.go",
			allowedImports: "os",
		},
		{
			pattern:        "test/harness/harness.go",
			allowedImports: "os",
		},
		{
			pattern:        "test/harness/ssh.go",
			allowedImports: "os,os/exec,net",
		},
		{
			pattern:        "cmd/scampi/fmt.go",
			allowedImports: "os",
		},
		{
			pattern:        "cmd/scampi/inspect.go",
			allowedImports: "os",
		},
		{
			pattern:        "cmd/scampi/secrets.go",
			allowedImports: "os",
		},
		{
			pattern:        "internal/step/sharedop/download_op.go",
			allowedImports: "crypto/md5, crypto/sha1, crypto/sha256, crypto/sha512, net/http",
		},
		{
			pattern:        "internal/step/pkg/pkg.go",
			allowedImports: "crypto/sha256",
		},
		{
			pattern:        "internal/step/unarchive/unarchive_op.go",
			allowedImports: "crypto/sha256",
		},
		{
			pattern:        "cmd/scampi/test.go",
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

	// matchImport checks if an import path matches a pattern.
	// Patterns ending in /* match any sub-import but not the base.
	// e.g., "os/*" matches "os/exec" but not "os"
	matchImport := func(pattern, importPath string) bool {
		if before, ok := strings.CutSuffix(pattern, "/*"); ok {
			base := before
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

		// Test files are free to import anything restricted.
		if strings.HasSuffix(rel, "_test.go") {
			return nil
		}

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

func Test_Rule_FuncSignatureStyle(t *testing.T) {
	root := repoRoot(t)

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
	"Printf", "Sprintf", "Errorf", "Fatalf", "Logf", "Skipf",
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
	return strings.HasSuffix(name, ".UnpackArgs") ||
		name == "UnpackArgs" ||
		name == "unpackArgs" ||
		name == "append" ||
		strings.HasSuffix(name, ".errAt")
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
		return // all on one line - fine
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

func Test_Rule_MarkdownTableAlignment(t *testing.T) {
	root := repoRoot(t)

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

	countPipes := func(s string) int {
		n := 0
		for i := 0; i < len(s); i++ {
			if s[i] == '|' && (i == 0 || s[i-1] != '\\') {
				n++
			}
		}
		return n
	}

	checkTable := func(t *testing.T, rel string, rows []string, startLine int) {
		t.Helper()
		if len(rows) < 2 {
			return
		}
		if !isSeparatorRow(rows[1]) {
			return
		}

		wantCols := countPipes(rows[0]) - 1
		wantLen := utf8.RuneCountInString(rows[0])

		for i, row := range rows {
			lineNum := startLine + i

			gotCols := countPipes(row) - 1
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

	// Walk site/content/
	for _, dir := range []string{"site/content"} {
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

	// Root-level markdown and the GitHub templates.
	rootDocs := []string{
		"README.md", "CLAUDE.md", "CONTRIBUTING.md", "SECURITY.md",
		"TERMINOLOGY.md",
		".github/PULL_REQUEST_TEMPLATE.md",
		".github/ISSUE_TEMPLATE/bug.md",
		".github/ISSUE_TEMPLATE/feature.md",
	}
	for _, doc := range rootDocs {
		p := filepath.Join(root, doc)
		if _, err := os.Stat(p); err == nil {
			walkFile(t, doc, p)
		}
	}
}

// Bare error constructors
// -----------------------------------------------------------------------------
//
// fmt.Errorf and errors.New produce errors without diagnostic IDs, source
// spans, or hints. In user-facing code they cause BUG panics when they leak
// through the engine boundary (panicIfNotAbortError). Use typed error structs
// implementing diagnostic.Diagnostic, or errs.BUG for invariant violations.
//
// errs.Errorf is allowed when a rationale comment appears on the line
// immediately above the call. Internal packages (target, secret, etc.) are
// sanctioned for fmt.Errorf/errors.New because their errors are wrapped by
// op/step code before reaching the engine.

func Test_Rule_BareErrorBan(t *testing.T) {
	root := repoRoot(t)

	// Always banned - use typed errors or errs.BUG instead.
	hardBanned := []string{
		"fmt.Errorf",
		"errors.New",
	}

	// Sanctioned files: internal helpers whose fmt.Errorf/errors.New
	// errors are wrapped before reaching the engine.
	sanctioned := []string{
		"internal/lang/resolve/resolve.go",     // internal FS errors wrapped into resolve.Error
		"internal/target/ssh/shell_session.go", // framing errors wrapped by run() into errSession
	}

	usedSanctions := map[string]bool{}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		// errs/ defines the wrappers - always exempt
		if strings.HasPrefix(rel, "internal/errs/") {
			return nil
		}
		// test/ is test infrastructure - always exempt
		if strings.HasPrefix(rel, "test/") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			name := ident.Name + "." + sel.Sel.Name
			pos := fset.Position(call.Pos())

			// errs.WrapErrf is sanctioned throughout internal/target/: the
			// managed-environment surface wraps OS/transport errors with
			// context, and ops surface them through typed diagnostics that
			// carry the ID and span. Anywhere else (linker, engine, steps)
			// it needs the same rationale comment as errs.Errorf - an
			// unwrapped WrapErrf on a user-facing path reaches the user
			// without ID, hint, or span (see #441).
			if name == "errs.WrapErrf" && strings.HasPrefix(rel, "internal/target/") {
				return true
			}

			// errs.Errorf, errs.New, and errs.WrapErrf are allowed with a
			// rationale comment on the preceding line, or above an
			// enclosing var/const block.
			if name == "errs.Errorf" || name == "errs.New" || name == "errs.WrapErrf" {
				if hasRationaleComment(file, fset, pos.Line-1) ||
					hasRationaleAboveBlock(file, fset, pos.Line) {
					return true
				}
				t.Errorf(
					"%s:%d: %s requires a \"// %s ...\" comment on the line above"+
						" (or above the enclosing var/const block)",
					rel, pos.Line, name, bareErrorRationale,
				)
				return true
			}

			if !slices.Contains(hardBanned, name) {
				return true
			}

			if slices.Contains(sanctioned, rel) {
				usedSanctions[rel] = true
				return true
			}

			t.Errorf(
				"%s:%d: %s produces bare errors without diagnostic IDs or source spans; "+
					"use a typed error implementing diagnostic.Diagnostic, or errs.BUG for invariant violations",
				rel, pos.Line, name,
			)
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Flag stale sanctions
	for _, s := range sanctioned {
		if !usedSanctions[s] {
			t.Errorf("sanctioned file %q no longer uses bare error constructors (remove from sanctioned list)", s)
		}
	}
}

const bareErrorRationale = "bare-error:"

func hasRationaleComment(file *ast.File, fset *token.FileSet, line int) bool {
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if fset.Position(c.Pos()).Line == line {
				text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
				if strings.HasPrefix(text, bareErrorRationale) {
					return true
				}
			}
		}
	}
	return false
}

// hasRationaleAboveBlock checks whether the call at callLine sits inside a
// var() or const() block that has a bare-error: comment on the line above
// its opening paren.
func hasRationaleAboveBlock(file *ast.File, fset *token.FileSet, callLine int) bool {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || (gd.Tok != token.VAR && gd.Tok != token.CONST) {
			continue
		}
		if gd.Lparen == 0 {
			continue // not a grouped block
		}
		openLine := fset.Position(gd.Lparen).Line
		closeLine := fset.Position(gd.Rparen).Line
		if callLine >= openLine && callLine <= closeLine {
			return hasRationaleComment(file, fset, openLine-1)
		}
	}
	return false
}

// Test_Rule_GlyphDiscipline bans non-ASCII bytes in Go source - literals AND
// comments - across internal/, cmd/, and test/. CLI glyphs go through the
// glyphSet in render/cli/glyph.go (fancy + ASCII variants); message prose is
// plain ASCII ("1-65535", "->", " - "); functional unicode in tests uses
// escape sequences (like \u2026) so the source file stays ASCII (#442).
//
// Exempt: glyph.go (the canonical glyph source) and token/pos_test.go
// (UTF-8 byte-offset arithmetic is unreadable as escapes).
func Test_Rule_GlyphDiscipline(t *testing.T) {
	roots := []string{"../../internal", "../../cmd", "../../test"}
	exempt := map[string]bool{
		"internal/render/cli/glyph.go":    true,
		"internal/lang/token/pos_test.go": true,
	}

	repo := repoRoot(t)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") {
				return nil
			}
			abs, aerr := filepath.Abs(p)
			if aerr != nil {
				return aerr
			}
			rel, rerr := filepath.Rel(repo, abs)
			if rerr != nil {
				return rerr
			}
			rel = filepath.ToSlash(rel)
			if exempt[rel] {
				return nil
			}

			data, rderr := os.ReadFile(p)
			if rderr != nil {
				return rderr
			}
			for i, line := range strings.Split(string(data), "\n") {
				for _, r := range line {
					if r > 127 {
						t.Errorf(
							"%s:%d: non-ASCII rune %q - the codebase is ASCII-only: glyphs go "+
								"through render/cli/glyph.go, prose uses ASCII punctuation, and "+
								"functional unicode in tests uses escape sequences",
							rel, i+1, r,
						)
						break
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Test file anchoring
// -----------------------------------------------------------------------------

// Test_Rule_NoOrphanTestFiles: in packages with prod code, every foo_test.go
// must sit next to a foo.go. An orphaned test file either tests code that
// lives elsewhere (rename or merge it) or marks a missing prod-file boundary
// (split the prod file). Packages without any prod file (pure-test packages
// like lang/test, and the whole test/ tree) have nothing to anchor to and are
// exempt.
func Test_Rule_NoOrphanTestFiles(t *testing.T) {
	roots := []string{"../../internal", "../../cmd"}

	// Build-tagged environment variants: tags apply per-file, so these cannot
	// merge into their anchor's test file. Keep this list painfully short.
	exempt := map[string]bool{
		"internal/target/local/escalate_nonroot_test.go": true, // !runasroot twin of escalate_test.go
	}

	repo := repoRoot(t)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, "_test.go") {
				return nil
			}
			abs, aerr := filepath.Abs(p)
			if aerr != nil {
				return aerr
			}
			rel, rerr := filepath.Rel(repo, abs)
			if rerr != nil {
				return rerr
			}
			if exempt[filepath.ToSlash(rel)] {
				return nil
			}
			if !dirHasProdGo(t, filepath.Dir(p)) {
				return nil
			}
			prod := strings.TrimSuffix(p, "_test.go") + ".go"
			if _, serr := os.Stat(prod); serr == nil {
				return nil
			}
			t.Errorf(
				"%s: orphaned test file (no %s) - merge it into the test file of the "+
					"prod file it exercises, or split that prod file at a real boundary",
				rel, filepath.Base(prod),
			)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// dirHasProdGo reports whether dir contains at least one non-test .go file.
func dirHasProdGo(t *testing.T, dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
}

// Test naming
// -----------------------------------------------------------------------------

// Test_Rule_TestNaming: test and benchmark functions are named
// Test_Subject_Expectation / Benchmark_Subject_Expectation - exactly two
// UpperCamel segments, so every test declares what it tests and what it
// expects. Enforcement rules use the Rule subject (Test_Rule_GlyphDiscipline),
// which fits the same shape.
//
// The expectation must actually EXPECT something: it needs a verb from the
// vocabulary below, so scenario labels don't masquerade as expectations
// (Test_SSH_RejectsWrongKey, not Test_SSH_ConnectWrongKey). A genuinely new
// verb fails here on purpose - add it to the list after checking the name
// states an asserted outcome, not an input.
//
// Exempt: TestMain (harness protocol), Fuzz* (no expectation to state),
// Benchmark_* (they measure a scenario, they don't assert), and Test_Rule_*
// (the rule name is the payload).
func Test_Rule_TestNaming(t *testing.T) {
	roots := []string{"../../internal", "../../cmd", "../../test"}
	nameRe := regexp.MustCompile(`^(Test|Benchmark)(_[A-Z][A-Za-z0-9]*){2}$`)

	fset := token.NewFileSet()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, p, nil, 0)
			if perr != nil {
				return perr
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				name := fn.Name.Name
				isTest := strings.HasPrefix(name, "Test")
				isBench := strings.HasPrefix(name, "Benchmark")
				if !isTest && !isBench {
					continue
				}
				if name == "TestMain" {
					continue
				}
				pos := fset.Position(fn.Pos())
				if !nameRe.MatchString(name) {
					t.Errorf(
						"%s:%d: %s does not match Test_Subject_Expectation "+
							"(exactly two UpperCamel segments)",
						p, pos.Line, name,
					)
					continue
				}
				if isBench || strings.HasPrefix(name, "Test_Rule_") {
					continue
				}
				expectation := name[strings.LastIndex(name, "_")+1:]
				if !containsExpectationVerb(expectation) {
					t.Errorf(
						"%s:%d: %s names a scenario, not an expectation - state the "+
							"asserted outcome with a verb (Test_SSH_RejectsWrongKey, not "+
							"Test_SSH_ConnectWrongKey); if the verb is genuinely new, add "+
							"it to expectationVerbs",
						p, pos.Line, name,
					)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// expectationVerbs is the third-person-singular vocabulary an expectation
// segment must draw from. Deliberately curated: a miss forces the question
// "am I naming what the test asserts, or just its input?".
var expectationVerbs = []string{
	"Aborts", "Accepts", "Adds", "Aggregates",
	"Aligns", "Allows", "Anchors", "Applies",
	"Assigns", "Bans", "Becomes", "Binds",
	"Blocks", "Bootstraps", "Bounds", "Buffers",
	"Caches", "Calls", "Canonicalizes", "Caps",
	"Carries", "Chains", "Classifies", "Clears",
	"Collapses", "Compiles", "Computes", "Connects",
	"Continues", "Converges", "Copies", "Counts",
	"Creates", "Decrypts", "Dedents", "Dedupes", "Defers",
	"Deletes", "Detects", "Disables", "Discards",
	"Does", "Drains", "Draws", "Drops",
	"Elides", "Emits", "Enforces", "Errors",
	"Escapes", "Evaluates", "Executes", "Exports",
	"Extracts", "Fails", "Falls", "Fences",
	"Fills", "Filters", "Finds", "Fires",
	"Fits", "Forces", "Formats", "Forwards",
	"Handles", "Honors", "Ignores", "Includes",
	"Inserts", "Is", "Keeps", "Leaves", "Limits",
	"Loads", "Maps", "Matches", "Mirrors",
	"Mixes", "Mounts", "Narrows", "Needs",
	"Normalizes", "Omits", "Orders", "Overrides",
	"Panics", "Parallelizes", "Parses", "Passes",
	"Picks", "Populates", "Prepends", "Preserves",
	"Quotes",
	"Prevents", "Probes", "Propagates", "Recreates",
	"Redacts", "Redraws", "Registers", "Rejects",
	"Relativizes", "Releases", "Remounts", "Removes",
	"Renders", "Reports", "Requires", "Resolves",
	"Respects", "Returns", "Rewrites", "Runs",
	"Searches", "Seeds", "Separates", "Serializes",
	"Sets", "Shows", "Skips", "Sorts",
	"Stays", "Stops", "Succeeds", "Sums",
	"Survives", "Tracks", "Treats", "Triggers",
	"Trims", "Trips", "Unescapes", "Unmounts", "Unwraps",
	"Updates", "Uses", "Verifies", "Waits",
	"Walks", "Warns", "Wins", "Wraps",
	"Writes", "Yields",
}

// containsExpectationVerb splits an UpperCamel expectation into words and
// reports whether any is a known verb.
func containsExpectationVerb(expectation string) bool {
	word := ""
	for _, r := range expectation {
		if r >= 'A' && r <= 'Z' && word != "" {
			if slices.Contains(expectationVerbs, word) {
				return true
			}
			word = string(r)
			continue
		}
		word += string(r)
	}
	return slices.Contains(expectationVerbs, word)
}
