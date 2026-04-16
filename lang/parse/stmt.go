// SPDX-License-Identifier: GPL-3.0-only

package parse

import (
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
)

// parseBlock parses statements until it sees RBrace (does NOT consume
// the RBrace itself — caller expects it).
func (p *Parser) parseBlock() *ast.Block {
	start := p.cur.Pos
	var stmts []ast.Stmt
	for p.cur.Kind != token.RBrace && p.cur.Kind != token.EOF {
		if p.cur.Kind == token.Semi {
			p.advance()
			continue
		}
		s := p.parseStmt()
		if s == nil {
			p.synchronize()
			continue
		}
		stmts = append(stmts, s)
	}
	end := p.cur.End
	return &ast.Block{
		Stmts:   stmts,
		SrcSpan: token.Span{Start: start, End: end},
	}
}

// parseStmt parses a single statement.
func (p *Parser) parseStmt() ast.Stmt {
	switch p.cur.Kind {
	case token.Let:
		d := p.parseLetDecl()
		if d == nil {
			return nil
		}
		return &ast.LetStmt{Decl: d}
	case token.For:
		return p.parseForStmt()
	case token.If:
		return p.parseIfStmt()
	case token.Return:
		return p.parseReturnStmt()
	}
	// Expression or assignment.
	e := p.parseExpr()
	if e == nil {
		return nil
	}
	// Assignment: target = value  (target must be IndexExpr or SelectorExpr)
	if p.cur.Kind == token.Assign {
		p.advance()
		val := p.parseExpr()
		if val == nil {
			return nil
		}
		if !isAssignable(e) {
			p.errAt(e.Span(), CodeNotAssignable, "left side of assignment is not assignable")
		}
		end := val.Span().End
		if p.cur.Kind == token.Semi {
			end = p.cur.End
			p.advance()
		}
		return &ast.AssignStmt{
			Target:  e,
			Value:   val,
			SrcSpan: token.Span{Start: e.Span().Start, End: end},
		}
	}
	if p.cur.Kind == token.Semi {
		p.advance()
	}
	return &ast.ExprStmt{Expr: e}
}

// isAssignable reports whether an expression can be the target of an
// assignment. Only IndexExpr and SelectorExpr are assignable in v0
// (mutating collection contents inside func bodies).
func isAssignable(e ast.Expr) bool {
	switch e.(type) {
	case *ast.IndexExpr, *ast.SelectorExpr:
		return true
	}
	return false
}

// parseForStmt:  for name in iter { body }
func (p *Parser) parseForStmt() *ast.ForStmt {
	start := p.cur.Pos
	p.advance() // 'for'
	v := p.parseIdent("for-loop variable")
	if v == nil {
		return nil
	}
	p.expect(token.In, "for-loop")
	// Iterator expression is parsed in condition mode so `xs { ... }`
	// is not interpreted as a struct literal.
	prev := p.inCond
	p.inCond = true
	iter := p.parseExpr()
	p.inCond = prev
	if iter == nil {
		return nil
	}
	p.expect(token.LBrace, "for-loop body")
	body := p.parseBlock()
	endTok := p.expect(token.RBrace, "for-loop body")
	if p.cur.Kind == token.Semi {
		p.advance()
	}
	return &ast.ForStmt{
		Var:     v,
		Iter:    iter,
		Body:    body,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}

// parseIfStmt:  if cond { then } [else { else_ }]
func (p *Parser) parseIfStmt() *ast.IfStmt {
	start := p.cur.Pos
	p.advance() // 'if'
	prev := p.inCond
	p.inCond = true
	cond := p.parseExpr()
	p.inCond = prev
	if cond == nil {
		return nil
	}
	p.expect(token.LBrace, "if body")
	thenB := p.parseBlock()
	endTok := p.expect(token.RBrace, "if body")
	var elseB *ast.Block
	if p.cur.Kind == token.Else {
		p.advance()
		// "else if" chains: rewrite as nested if inside an Else block.
		if p.cur.Kind == token.If {
			inner := p.parseIfStmt()
			elseB = &ast.Block{
				Stmts:   []ast.Stmt{inner},
				SrcSpan: inner.Span(),
			}
			endTok.End = inner.Span().End
		} else {
			p.expect(token.LBrace, "else body")
			elseB = p.parseBlock()
			endTok = p.expect(token.RBrace, "else body")
		}
	}
	if p.cur.Kind == token.Semi {
		p.advance()
	}
	return &ast.IfStmt{
		Cond:    cond,
		Then:    thenB,
		Else:    elseB,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}

// parseReturnStmt:  return [expr]
func (p *Parser) parseReturnStmt() *ast.ReturnStmt {
	start := p.cur.Pos
	p.advance() // 'return'
	var v ast.Expr
	end := start + 6 // "return"
	if p.cur.Kind != token.Semi && p.cur.Kind != token.RBrace && p.cur.Kind != token.EOF {
		v = p.parseExpr()
		if v != nil {
			end = v.Span().End
		}
	}
	if p.cur.Kind == token.Semi {
		end = p.cur.End
		p.advance()
	}
	return &ast.ReturnStmt{
		Value:   v,
		SrcSpan: token.Span{Start: start, End: end},
	}
}
