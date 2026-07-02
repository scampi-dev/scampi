// SPDX-License-Identifier: GPL-3.0-only

package ast_test

import (
	"reflect"
	"testing"

	"scampi.dev/scampi/internal/lang/ast"
	"scampi.dev/scampi/internal/lang/lex"
	"scampi.dev/scampi/internal/lang/parse"
)

// walkSource packs every parser-producible node type into one file.
const walkSource = `module main

import "std"

type @tag {
  reason: string
}

type Server {
  @tag(reason = "primary")
  name: string
  port: int = 8080
  aliases: list[string]
  parent: string?
}

enum Color {
  red
  green
}

func scale(values: list[int], factor: int = 2) list[int] {
  return [v * factor for v in values if v > 0]
}

decl my.step(name: string) Step {
  let who = self
}

func demo(flag: bool) int {
  let pair = {"a": 1}
  pair["a"] = 2
  if flag == true {
    return 1
  } else {
    pair["a"] = 0 - 1
  }
  for v in [1, 2] {
    pair["b"] = v
  }
  let neg = !false
  let pick = if neg { 1 } else { 2 }
  let empty = none
  let one = pair["a"]
  let par = (one)
  let msg = "port ${par}"
  let srv = Server { name = "web" }
  let alias = srv.name
  let doubled = scale([3], factor = 2)
  return doubled[0]
}

std.deploy(name = "d", targets = []) {
  my.step { name = "s" }
}
`

func Test_Walk_YieldsEveryNodeType(t *testing.T) {
	l := lex.New("walk_test.scampi", []byte(walkSource))
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		t.Fatalf("lex errors: %v", errs)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}

	seen := map[string]int{}
	count := func(n ast.Node) bool {
		seen[reflect.TypeOf(n).Elem().Name()]++
		return true
	}
	post := 0
	ast.Walk(f, count, func(ast.Node) { post++ })

	// MapComp has no surface syntax yet; walk a hand-built node so its
	// traversal path is covered too.
	mapComp := &ast.MapComp{
		Key:   &ast.StringLit{},
		Value: &ast.Ident{Name: "v"},
		Vars:  []*ast.Ident{{Name: "k"}, {Name: "v"}},
		Iter:  &ast.Ident{Name: "items"},
		Cond:  &ast.BoolLit{Value: true},
	}
	ast.Walk(mapComp, count, nil)

	want := []string{
		"File", "ModuleDecl", "ImportDecl",
		"TypeDecl", "AttrTypeDecl", "EnumDecl", "FuncDecl", "DeclDecl", "LetDecl",
		"LetStmt", "ForStmt", "IfStmt", "ReturnStmt", "ExprStmt", "AssignStmt", "Block",
		"Ident", "DottedName", "ParenExpr",
		"IntLit", "StringLit", "BoolLit", "NoneLit", "SelfLit",
		"ListLit", "MapLit", "StructLit", "BlockExpr",
		"CallExpr", "SelectorExpr", "IndexExpr",
		"BinaryExpr", "UnaryExpr", "IfExpr", "ListComp", "MapComp",
		"NamedType", "GenericType", "OptionalType",
		"Attribute", "AttrArg",
	}
	for _, name := range want {
		if seen[name] == 0 {
			t.Errorf("node type %s never visited", name)
		}
	}
	if post == 0 {
		t.Error("post callback never fired")
	}
}
