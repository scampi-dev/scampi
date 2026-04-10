// SPDX-License-Identifier: GPL-3.0-only

package ast

// Walk traverses the AST rooted at node in depth-first order. pre is
// called before visiting a node's children; post is called after. If
// pre returns false, children and post are skipped for that subtree.
// Either callback may be nil.
//
// Walk is the single place that encodes "what are the children of each
// node type, in what order." Consumers (type checker, LSP, formatter,
// evaluator) supply the logic via callbacks — they never need to know
// about traversal order.
func Walk(node Node, pre func(Node) bool, post func(Node)) {
	if node == nil {
		return
	}
	if pre != nil && !pre(node) {
		return
	}
	walkChildren(node, pre, post)
	if post != nil {
		post(node)
	}
}

func walkChildren(node Node, pre func(Node) bool, post func(Node)) {
	switch n := node.(type) {

	// Top-level
	case *File:
		if n.Module != nil {
			Walk(n.Module, pre, post)
		}
		walkList(n.Imports, pre, post)
		walkDeclList(n.Decls, pre, post)
		walkStmtList(n.Stmts, pre, post)

	case *ModuleDecl:
		Walk(n.Name, pre, post)

	case *ImportDecl:
		// no children

	case *TypeDecl:
		Walk(n.Name, pre, post)
		walkFieldList(n.Fields, pre, post)

	case *AttrTypeDecl:
		Walk(n.Name, pre, post)
		walkFieldList(n.Fields, pre, post)

	case *EnumDecl:
		Walk(n.Name, pre, post)
		walkList(n.Variants, pre, post)

	case *FuncDecl:
		Walk(n.Name, pre, post)
		walkFieldList(n.Params, pre, post)
		walkTypeExpr(n.Ret, pre, post)
		if n.Body != nil {
			Walk(n.Body, pre, post)
		}

	case *DeclDecl:
		Walk(n.Name, pre, post)
		walkFieldList(n.Params, pre, post)
		walkTypeExpr(n.Ret, pre, post)
		if n.Body != nil {
			Walk(n.Body, pre, post)
		}

	case *LetDecl:
		Walk(n.Name, pre, post)
		walkTypeExpr(n.Type, pre, post)
		walkExpr(n.Value, pre, post)

	case *LetStmt:
		Walk(n.Decl, pre, post)

	case *ForStmt:
		Walk(n.Var, pre, post)
		walkExpr(n.Iter, pre, post)
		Walk(n.Body, pre, post)

	case *IfStmt:
		walkExpr(n.Cond, pre, post)
		Walk(n.Then, pre, post)
		Walk(n.Else, pre, post)

	case *ReturnStmt:
		walkExpr(n.Value, pre, post)

	case *ExprStmt:
		walkExpr(n.Expr, pre, post)

	case *AssignStmt:
		walkExpr(n.Target, pre, post)
		walkExpr(n.Value, pre, post)

	case *Block:
		walkStmtList(n.Stmts, pre, post)

	case *Ident:
		// no children

	case *DottedName:
		walkList(n.Parts, pre, post)

	case *IntLit:
		// no children

	case *StringLit:
		for _, p := range n.Parts {
			switch p := p.(type) {
			case *StringText:
				_ = p
			case *StringInterp:
				walkExpr(p.Expr, pre, post)
			}
		}

	case *BoolLit, *NoneLit, *SelfLit:
		// no children

	case *ListLit:
		walkExprList(n.Items, pre, post)

	case *MapLit:
		for _, e := range n.Entries {
			walkExpr(e.Key, pre, post)
			walkExpr(e.Value, pre, post)
		}

	case *StructLit:
		walkTypeExpr(n.Type, pre, post)
		for _, f := range n.Fields {
			Walk(f.Name, pre, post)
			walkExpr(f.Value, pre, post)
		}
		walkStmtList(n.Body, pre, post)

	case *BlockExpr:
		walkExpr(n.Target, pre, post)
		Walk(n.Body, pre, post)

	case *CallExpr:
		walkExpr(n.Fn, pre, post)
		for _, a := range n.Args {
			if a.Name != nil {
				Walk(a.Name, pre, post)
			}
			walkExpr(a.Value, pre, post)
		}

	case *SelectorExpr:
		walkExpr(n.X, pre, post)
		Walk(n.Sel, pre, post)

	case *IndexExpr:
		walkExpr(n.X, pre, post)
		walkExpr(n.Index, pre, post)

	case *BinaryExpr:
		walkExpr(n.Left, pre, post)
		walkExpr(n.Right, pre, post)

	case *UnaryExpr:
		walkExpr(n.X, pre, post)

	case *IfExpr:
		walkExpr(n.Cond, pre, post)
		walkExpr(n.Then, pre, post)
		walkExpr(n.Else, pre, post)

	case *ListComp:
		walkExpr(n.Expr, pre, post)
		Walk(n.Var, pre, post)
		walkExpr(n.Iter, pre, post)
		walkExpr(n.Cond, pre, post)

	case *MapComp:
		walkExpr(n.Key, pre, post)
		walkExpr(n.Value, pre, post)
		walkList(n.Vars, pre, post)
		walkExpr(n.Iter, pre, post)
		walkExpr(n.Cond, pre, post)

	// Type expressions
	case *NamedType:
		Walk(n.Name, pre, post)

	case *GenericType:
		Walk(n.Name, pre, post)
		for _, a := range n.Args {
			walkTypeExpr(a, pre, post)
		}

	case *OptionalType:
		walkTypeExpr(n.Inner, pre, post)

	case *Attribute:
		Walk(n.Name, pre, post)
		walkExprList(n.Positionals, pre, post)
		for _, a := range n.Named {
			Walk(a, pre, post)
		}

	case *AttrArg:
		Walk(n.Name, pre, post)
		walkExpr(n.Value, pre, post)
	}
}

// Helpers that handle nil-checks and interface→concrete dispatching.

func walkExpr(e Expr, pre func(Node) bool, post func(Node)) {
	if e != nil {
		Walk(e, pre, post)
	}
}

func walkTypeExpr(t TypeExpr, pre func(Node) bool, post func(Node)) {
	if t != nil {
		Walk(t, pre, post)
	}
}

func walkExprList(exprs []Expr, pre func(Node) bool, post func(Node)) {
	for _, e := range exprs {
		Walk(e, pre, post)
	}
}

func walkDeclList(decls []Decl, pre func(Node) bool, post func(Node)) {
	for _, d := range decls {
		Walk(d, pre, post)
	}
}

func walkStmtList(stmts []Stmt, pre func(Node) bool, post func(Node)) {
	for _, s := range stmts {
		Walk(s, pre, post)
	}
}

func walkFieldList(fields []*Field, pre func(Node) bool, post func(Node)) {
	for _, f := range fields {
		for _, a := range f.Attributes {
			Walk(a, pre, post)
		}
		Walk(f.Name, pre, post)
		walkTypeExpr(f.Type, pre, post)
		walkExpr(f.Default, pre, post)
	}
}

func walkList[T Node](items []T, pre func(Node) bool, post func(Node)) {
	for _, item := range items {
		Walk(item, pre, post)
	}
}
