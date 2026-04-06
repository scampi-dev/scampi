// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
)

// Expected
// -----------------------------------------------------------------------------

type expected struct {
	Imports []string     `json:"imports,omitempty"`
	Decls   []expectDecl `json:"decls,omitempty"`
	Stmts   int          `json:"stmts,omitempty"`
	Errors  []string     `json:"errors"`
}

type expectDecl struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Fields   int    `json:"fields,omitempty"`
	Variants int    `json:"variants,omitempty"`
	Params   int    `json:"params,omitempty"`
}

// Runner
// -----------------------------------------------------------------------------

func TestLangFixtures(t *testing.T) {
	root := "testdata"
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".scampi") {
			return nil
		}
		jsonPath := strings.TrimSuffix(p, ".scampi") + ".json"
		if _, err := os.Stat(jsonPath); err != nil {
			return nil // no expected file, skip
		}

		rel, _ := filepath.Rel(root, p)
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".scampi")

		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			expBytes, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatal(err)
			}
			var exp expected
			if err := json.Unmarshal(expBytes, &exp); err != nil {
				t.Fatalf("bad expected JSON: %v", err)
			}
			runFixture(t, string(src), exp)
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runFixture(t *testing.T, src string, exp expected) {
	t.Helper()

	l := lex.New("test.scampi", []byte(src))
	p := parse.New(l)
	f := p.Parse()

	var allErrs []string
	for _, e := range l.Errors() {
		allErrs = append(allErrs, e.Error())
	}
	for _, e := range p.Errors() {
		allErrs = append(allErrs, e.Error())
	}

	c := check.New()
	c.Check(f)
	for _, e := range c.Errors() {
		allErrs = append(allErrs, e.Error())
	}

	checkImports(t, f, exp.Imports)
	checkDecls(t, f, exp.Decls)
	checkStmts(t, f, exp.Stmts)
	checkErrors(t, allErrs, exp.Errors)
}

// Assertions
// -----------------------------------------------------------------------------

func checkImports(t *testing.T, f *ast.File, want []string) {
	t.Helper()
	if want == nil {
		return
	}
	if len(f.Imports) != len(want) {
		t.Errorf("imports: got %d, want %d", len(f.Imports), len(want))
		return
	}
	for i, w := range want {
		if f.Imports[i].Path != w {
			t.Errorf("import[%d]: got %q, want %q", i, f.Imports[i].Path, w)
		}
	}
}

func checkDecls(t *testing.T, f *ast.File, want []expectDecl) {
	t.Helper()
	if want == nil {
		return
	}
	if len(f.Decls) != len(want) {
		t.Errorf("decls: got %d, want %d", len(f.Decls), len(want))
		return
	}
	for i, w := range want {
		d := f.Decls[i]
		kind, name := declKindName(d)
		if kind != w.Kind {
			t.Errorf("decl[%d] kind: got %q, want %q", i, kind, w.Kind)
		}
		if name != w.Name {
			t.Errorf("decl[%d] name: got %q, want %q", i, name, w.Name)
		}
		switch dd := d.(type) {
		case *ast.StructDecl:
			if w.Fields > 0 && len(dd.Fields) != w.Fields {
				t.Errorf("decl[%d] fields: got %d, want %d", i, len(dd.Fields), w.Fields)
			}
		case *ast.EnumDecl:
			if w.Variants > 0 && len(dd.Variants) != w.Variants {
				t.Errorf("decl[%d] variants: got %d, want %d", i, len(dd.Variants), w.Variants)
			}
		case *ast.StepDecl:
			if w.Params > 0 && len(dd.Params) != w.Params {
				t.Errorf("decl[%d] params: got %d, want %d", i, len(dd.Params), w.Params)
			}
		case *ast.FuncDecl:
			if w.Params > 0 && len(dd.Params) != w.Params {
				t.Errorf("decl[%d] params: got %d, want %d", i, len(dd.Params), w.Params)
			}
		}
	}
}

func checkStmts(t *testing.T, f *ast.File, want int) {
	t.Helper()
	if want > 0 && len(f.Stmts) != want {
		t.Errorf("stmts: got %d, want %d", len(f.Stmts), want)
	}
}

func checkErrors(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(want) == 0 {
		if len(got) > 0 {
			t.Errorf("expected no errors, got: %v", got)
		}
		return
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if strings.Contains(g, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error containing %q, got: %v", w, got)
		}
	}
}

func declKindName(d ast.Decl) (string, string) {
	switch d := d.(type) {
	case *ast.StructDecl:
		return "struct", d.Name.Name
	case *ast.EnumDecl:
		return "enum", d.Name.Name
	case *ast.FuncDecl:
		return "func", d.Name.Name
	case *ast.StepDecl:
		return "step", d.Name.Parts[0].Name
	case *ast.LetDecl:
		return "let", d.Name.Name
	}
	return "unknown", ""
}
