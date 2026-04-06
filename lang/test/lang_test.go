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
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
)

// Shared fixture walker
// -----------------------------------------------------------------------------

func walkFixtures(t *testing.T, root string, fn func(*testing.T, string, []byte)) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".scampi") {
			return nil
		}
		jsonPath := strings.TrimSuffix(p, ".scampi") + ".json"
		if _, err := os.Stat(jsonPath); err != nil {
			return nil
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
			fn(t, string(src), expBytes)
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Parse tests — verify AST shape
// -----------------------------------------------------------------------------

type parseExpected struct {
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

func TestParse(t *testing.T) {
	walkFixtures(t, "testdata/parse", func(t *testing.T, src string, expBytes []byte) {
		t.Helper()
		var exp parseExpected
		if err := json.Unmarshal(expBytes, &exp); err != nil {
			t.Fatalf("bad expected JSON: %v", err)
		}

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

		assertImports(t, f, exp.Imports)
		assertDecls(t, f, exp.Decls)
		assertStmtCount(t, f, exp.Stmts)
		assertErrors(t, allErrs, exp.Errors)
	})
}

// Error tests — verify diagnostics
// -----------------------------------------------------------------------------

func TestErrors(t *testing.T) {
	walkFixtures(t, "testdata/errors", func(t *testing.T, src string, expBytes []byte) {
		t.Helper()
		var exp parseExpected
		if err := json.Unmarshal(expBytes, &exp); err != nil {
			t.Fatalf("bad expected JSON: %v", err)
		}

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

		assertDecls(t, f, exp.Decls)
		assertErrors(t, allErrs, exp.Errors)
	})
}

// Eval tests — full pipeline → runtime values
// -----------------------------------------------------------------------------

type evalExpected struct {
	Lets    map[string]json.RawMessage `json:"lets,omitempty"`
	Targets []expectTarget             `json:"targets,omitempty"`
	Deploys []expectDeploy             `json:"deploys,omitempty"`
	Secrets *expectSecrets             `json:"secrets,omitempty"`
	Errors  []string                   `json:"errors,omitempty"`
}

type expectTarget struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type expectDeploy struct {
	Name  string `json:"name"`
	Steps int    `json:"steps,omitempty"`
}

type expectSecrets struct {
	Backend string `json:"backend"`
	Path    string `json:"path"`
}

func TestEval(t *testing.T) {
	walkFixtures(t, "testdata/eval", func(t *testing.T, src string, expBytes []byte) {
		t.Helper()
		var exp evalExpected
		if err := json.Unmarshal(expBytes, &exp); err != nil {
			t.Fatalf("bad expected JSON: %v", err)
		}

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
		if len(allErrs) > 0 && len(exp.Errors) == 0 {
			t.Fatalf("lex/parse errors: %v", allErrs)
		}

		c := check.New()
		c.Check(f)
		for _, e := range c.Errors() {
			allErrs = append(allErrs, e.Error())
		}
		if len(allErrs) > 0 && len(exp.Errors) == 0 {
			t.Fatalf("check errors: %v", allErrs)
		}

		r, errs := eval.Eval(f, []byte(src))
		for _, e := range errs {
			allErrs = append(allErrs, e.Error())
		}

		if len(exp.Errors) > 0 {
			assertErrors(t, allErrs, exp.Errors)
			return
		}
		if len(allErrs) > 0 {
			t.Fatalf("eval errors: %v", allErrs)
		}

		assertLets(t, r, exp.Lets)
		assertTargets(t, r, exp.Targets)
		assertDeploys(t, r, exp.Deploys)
		assertSecrets(t, r, exp.Secrets)
	})
}

// Eval value assertions
// -----------------------------------------------------------------------------

func assertLets(t *testing.T, r *eval.Result, want map[string]json.RawMessage) {
	t.Helper()
	if want == nil {
		return
	}
	for name, rawWant := range want {
		got, ok := r.Bindings[name]
		if !ok {
			t.Errorf("let %q: not found in eval result", name)
			continue
		}
		assertValueEquals(t, name, got, rawWant)
	}
}

func assertTargets(t *testing.T, r *eval.Result, want []expectTarget) {
	t.Helper()
	if want == nil {
		return
	}
	if len(r.Targets) != len(want) {
		t.Errorf("targets: got %d, want %d", len(r.Targets), len(want))
		return
	}
	for i, w := range want {
		if r.Targets[i].Name != w.Name {
			t.Errorf("target[%d] name: got %q, want %q", i, r.Targets[i].Name, w.Name)
		}
		if r.Targets[i].Kind != w.Kind {
			t.Errorf("target[%d] kind: got %q, want %q", i, r.Targets[i].Kind, w.Kind)
		}
	}
}

func assertDeploys(t *testing.T, r *eval.Result, want []expectDeploy) {
	t.Helper()
	if want == nil {
		return
	}
	if len(r.Deploys) != len(want) {
		t.Errorf("deploys: got %d, want %d", len(r.Deploys), len(want))
		return
	}
	for i, w := range want {
		if r.Deploys[i].Name != w.Name {
			t.Errorf("deploy[%d] name: got %q, want %q", i, r.Deploys[i].Name, w.Name)
		}
		if w.Steps > 0 && len(r.Deploys[i].Steps) != w.Steps {
			t.Errorf("deploy[%d] steps: got %d, want %d", i, len(r.Deploys[i].Steps), w.Steps)
		}
	}
}

func assertSecrets(t *testing.T, r *eval.Result, want *expectSecrets) {
	t.Helper()
	if want == nil {
		return
	}
	if r.Secrets == nil {
		t.Fatal("expected secrets config, got nil")
	}
	if r.Secrets.Backend != want.Backend {
		t.Errorf("secrets backend: got %q, want %q", r.Secrets.Backend, want.Backend)
	}
	if r.Secrets.Path != want.Path {
		t.Errorf("secrets path: got %q, want %q", r.Secrets.Path, want.Path)
	}
}

// Shared assertions
// -----------------------------------------------------------------------------

func assertImports(t *testing.T, f *ast.File, want []string) {
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

func assertDecls(t *testing.T, f *ast.File, want []expectDecl) {
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

func assertStmtCount(t *testing.T, f *ast.File, want int) {
	t.Helper()
	if want > 0 && len(f.Stmts) != want {
		t.Errorf("stmts: got %d, want %d", len(f.Stmts), want)
	}
}

func assertErrors(t *testing.T, got []string, want []string) {
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

// Value comparison helpers
// -----------------------------------------------------------------------------

func assertValueEquals(t *testing.T, name string, got eval.Value, rawWant json.RawMessage) {
	t.Helper()
	var want any
	if err := json.Unmarshal(rawWant, &want); err != nil {
		t.Fatalf("let %q: bad expected value: %v", name, err)
	}
	if !valueMatchesJSON(got, want) {
		t.Errorf("let %q: got %v, want %v", name, got, want)
	}
}

func valueMatchesJSON(v eval.Value, j any) bool {
	switch jv := j.(type) {
	case float64:
		if iv, ok := v.(*eval.IntVal); ok {
			return iv.V == int64(jv)
		}
	case string:
		if sv, ok := v.(*eval.StringVal); ok {
			return sv.V == jv
		}
	case bool:
		if bv, ok := v.(*eval.BoolVal); ok {
			return bv.V == jv
		}
	case nil:
		_, ok := v.(*eval.NoneVal)
		return ok
	case []any:
		lv, ok := v.(*eval.ListVal)
		if !ok || len(lv.Items) != len(jv) {
			return false
		}
		for i, item := range jv {
			if !valueMatchesJSON(lv.Items[i], item) {
				return false
			}
		}
		return true
	}
	return false
}
