// SPDX-License-Identifier: GPL-3.0-only

// Command bench-descriptions walks every `*_test.go` file in the
// repository, finds functions matching the Go testing benchmark
// signature `func Benchmark<Name>(b *testing.B, ...)`, and emits a
// JSON map of <Name> -> doc-comment text. The scampi.dev /bench/
// page consumes the JSON to render a one-line description per
// chart.
//
// Convention enforced: every benchmark function MUST have a doc
// comment. The generator errors out if any are missing so adding a
// new benchmark forces a description at generate time, not at site
// view time.
//
// Run via `just generate` or directly:
//
//	go run ./bin/bench-descriptions -o site/static/bench/descriptions.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"scampi.dev/scampi/errs"
)

func main() {
	out := flag.String("o", "site/data/benchmarks.json", "output JSON path")
	root := flag.String("root", ".", "root directory to walk")
	flag.Parse()

	descs, err := collect(*root)
	if err != nil {
		fail("collect: %v", err)
	}
	if len(descs) == 0 {
		fail("no benchmarks found under %s", *root)
	}
	if err := write(*out, descs); err != nil {
		fail("write %s: %v", *out, err)
	}
	_, _ = fmt.Fprintf(os.Stderr, "wrote %d descriptions to %s\n", len(descs), *out)
}

func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "bench-descriptions: "+format+"\n", args...)
	os.Exit(1)
}

// collect walks all *_test.go files under root and returns the doc
// comment of every benchmark function, keyed by the name with the
// "Benchmark" prefix stripped (matches how the dashboard JS keys
// charts off the family name).
func collect(root string) (map[string]string, error) {
	descs := make(map[string]string)
	var missing []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Don't skip the walk root itself, even if its name is "."
			// or starts with a dot (e.g. when run from inside a hidden
			// directory).
			if path == root {
				return nil
			}
			name := d.Name()
			// Skip vendored, generated, scratch, and submodule trees.
			if name == "vendor" || name == "node_modules" || name == "build" ||
				strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			// bare-error: build-time code-gen tool, output goes to stderr
			return errs.Errorf("%s: %v", path, perr)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if !isBenchmark(fn) {
				continue
			}
			key := strings.TrimPrefix(fn.Name.Name, "Benchmark")
			if fn.Doc == nil || strings.TrimSpace(fn.Doc.Text()) == "" {
				missing = append(missing, fmt.Sprintf("%s in %s", fn.Name.Name, path))
				continue
			}
			text := normalize(fn.Doc.Text())
			// godoc convention: doc starts with the function name as
			// subject ("BenchmarkX measures..."). The dashboard renders
			// the function name as the chart title, so trim the
			// repeated prefix.
			text = strings.TrimPrefix(text, fn.Name.Name+" ")
			descs[key] = text
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		// bare-error: build-time code-gen tool, output goes to stderr
		return nil, errs.Errorf(
			"benchmark functions without doc comment (add a // ... block above each):\n  %s",
			strings.Join(missing, "\n  "),
		)
	}
	return descs, nil
}

// isBenchmark matches `func Bench…(b *testing.B …)`. We don't restrict
// the suffix shape (e.g. Apply_NoOp_Mount is fine) — the doc-string
// matters, not the naming scheme.
func isBenchmark(fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		return false
	}
	if !strings.HasPrefix(fn.Name.Name, "Benchmark") {
		return false
	}
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return false
	}
	first := fn.Type.Params.List[0]
	star, ok := first.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "testing" && sel.Sel.Name == "B"
}

// camelCompounds lists tokens that look split-able under the naive
// "space before uppercase preceded by lowercase" rule but should stay
// joined. Add entries here when new benchmark names need them.
var camelCompounds = []string{"NoOp"}

// prettify turns a Go benchmark function name (already stripped of
// the "Benchmark" prefix) into a human title for the dashboard:
//
//	ApplyMixed_Cold              -> "Apply Mixed - Cold"
//	ApplyNoOp                    -> "Apply NoOp"
//	ApplyNoOp_Unarchive_TarGz    -> "Apply NoOp - Unarchive - Tar Gz"
//	ApplyNoOp_REST               -> "Apply NoOp - REST"
//	LoadConfig                   -> "Load Config"
func prettify(name string) string {
	segments := strings.Split(name, "_")
	for i, seg := range segments {
		segments[i] = prettifySegment(seg)
	}
	return strings.Join(segments, " - ")
}

func prettifySegment(seg string) string {
	if seg == "" {
		return seg
	}
	// All-uppercase segment (e.g. "REST") — treat as an acronym, no
	// internal splitting.
	if isAllUpper(seg) {
		return seg
	}
	var b strings.Builder
	runes := []rune(seg)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	out := b.String()
	// Restore known compounds that the naive split broke apart.
	for _, c := range camelCompounds {
		spaced := spaceCamel(c)
		if spaced != c {
			out = strings.ReplaceAll(out, spaced, c)
		}
	}
	return out
}

// spaceCamel applies the same naive-split rule to a compound so we
// know how it would look after prettifySegment broke it. Used to
// build the search/replace pair that rejoins it.
func spaceCamel(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isAllUpper(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// normalize collapses the godoc-style doc text into a single
// whitespace-separated string. Preserves sentence boundaries via the
// single space between lines.
func normalize(doc string) string {
	lines := strings.Split(strings.TrimSpace(doc), "\n")
	parts := make([]string, 0, len(lines))
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " ")
}

func write(path string, descs map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Deterministic key order so the file diffs cleanly when only
	// content changes.
	keys := make([]string, 0, len(descs))
	for k := range descs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(descs))
	for _, k := range keys {
		ordered[k] = descs[k]
	}
	b, err := json.MarshalIndent(orderedJSON(keys, descs), "", "  ")
	if err != nil {
		return err
	}
	// Ensure trailing newline (POSIX-friendly).
	if !strings.HasSuffix(string(b), "\n") {
		b = append(b, '\n')
	}
	return os.WriteFile(path, b, 0o644)
}

// orderedJSON returns the descs as a slice of {name, pretty, desc}
// objects preserving the given key order. (Go's encoding/json sorts
// map keys alphabetically already, but going through a slice makes
// the contract explicit.)
func orderedJSON(keys []string, descs map[string]string) any {
	type entry struct {
		Name   string `json:"name"`
		Pretty string `json:"pretty"`
		Desc   string `json:"desc"`
	}
	out := make([]entry, 0, len(keys))
	for _, k := range keys {
		out = append(out, entry{Name: k, Pretty: prettify(k), Desc: descs[k]})
	}
	return out
}
