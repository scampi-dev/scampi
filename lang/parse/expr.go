// SPDX-License-Identifier: GPL-3.0-only

package parse

import (
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
)

// Operator precedence. Higher number = tighter binding.
// -1 means "not a binary operator".
func binaryPrec(k token.Kind) int {
	switch k {
	case token.Or:
		return 1
	case token.And:
		return 2
	case token.Eq, token.Neq, token.Lt, token.Gt, token.Leq, token.Geq:
		return 3
	case token.In:
		return 4
	case token.Plus, token.Minus:
		return 5
	case token.Star, token.Slash, token.Percent:
		return 6
	}
	return -1
}

// parseExpr parses an expression using precedence climbing.
func (p *Parser) parseExpr() ast.Expr {
	return p.parseBinary(0)
}

// parseBinary parses a binary expression at minimum precedence minPrec.
func (p *Parser) parseBinary(minPrec int) ast.Expr {
	left := p.parseUnary()
	if left == nil {
		return nil
	}
	for {
		prec := binaryPrec(p.cur.Kind)
		if prec < minPrec {
			break
		}
		op := p.cur.Kind
		p.advance()
		right := p.parseBinary(prec + 1) // left-associative
		if right == nil {
			return left
		}
		left = &ast.BinaryExpr{
			Op:      op,
			Left:    left,
			Right:   right,
			SrcSpan: token.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}
	return left
}

// parseUnary parses a unary expression (!x, -x) or a postfix expression.
func (p *Parser) parseUnary() ast.Expr {
	if p.cur.Kind == token.Not || p.cur.Kind == token.Minus {
		op := p.cur.Kind
		start := p.cur.Pos
		p.advance()
		x := p.parseUnary()
		if x == nil {
			return nil
		}
		return &ast.UnaryExpr{
			Op:      op,
			X:       x,
			SrcSpan: token.Span{Start: start, End: x.Span().End},
		}
	}
	return p.parsePostfix()
}

// parsePostfix parses a primary expression followed by any sequence
// of .field, [index], and (args) suffixes.
func (p *Parser) parsePostfix() ast.Expr {
	x := p.parsePrimary()
	if x == nil {
		return nil
	}
	for {
		switch p.cur.Kind {
		case token.Dot:
			p.advance()
			sel := p.parseIdent("selector")
			if sel == nil {
				return x
			}
			x = &ast.SelectorExpr{
				X:       x,
				Sel:     sel,
				SrcSpan: token.Span{Start: x.Span().Start, End: sel.SrcSpan.End},
			}
		case token.LBrack:
			p.advance()
			idx := p.parseExpr()
			endTok := p.expect(token.RBrack, "index expression")
			x = &ast.IndexExpr{
				X:       x,
				Index:   idx,
				SrcSpan: token.Span{Start: x.Span().Start, End: endTok.End},
			}
		case token.LParen:
			p.advance()
			args := p.parseCallArgs()
			endTok := p.expect(token.RParen, "call arguments")
			x = &ast.CallExpr{
				Fn:      x,
				Args:    args,
				SrcSpan: token.Span{Start: x.Span().Start, End: endTok.End},
			}
		case token.LBrace:
			// Struct-literal call syntax: TypeName { field = value }
			// Only valid when the preceding expression is a name and
			// we're not currently parsing a control-flow condition
			// (where `{` always starts the body).
			if p.inCond || !canTakeStructLit(x) {
				return x
			}
			x = p.parseStructLitBody(x)
		default:
			return x
		}
	}
}

// canTakeStructLit reports whether an expression can be followed by
// { field = value, ... } to form a struct literal invocation.
// Limited to identifiers and dotted names (type/step references).
func canTakeStructLit(x ast.Expr) bool {
	switch x.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.DottedName:
		return true
	}
	return false
}

// parseStructLitBody parses the { field = value, ... } suffix after
// an identifier/selector, producing a StructLit with the caller's
// expression as the type.
func (p *Parser) parseStructLitBody(typeExpr ast.Expr) ast.Expr {
	start := typeExpr.Span().Start
	p.advance() // '{'
	fields, body := p.parseBlockContent(token.RBrace)
	endTok := p.expect(token.RBrace, "struct/step literal body")
	typ := exprToTypeExpr(typeExpr)
	return &ast.StructLit{
		Type:    typ,
		Fields:  fields,
		Body:    body,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}

// exprToTypeExpr converts an Ident/SelectorExpr/DottedName expression
// into a TypeExpr suitable for a StructLit's Type field.
func exprToTypeExpr(x ast.Expr) ast.TypeExpr {
	switch v := x.(type) {
	case *ast.Ident:
		name := &ast.DottedName{Parts: []*ast.Ident{v}, SrcSpan: v.SrcSpan}
		return &ast.NamedType{Name: name, SrcSpan: v.SrcSpan}
	case *ast.DottedName:
		return &ast.NamedType{Name: v, SrcSpan: v.SrcSpan}
	case *ast.SelectorExpr:
		// Flatten selector chain into a dotted name where possible.
		parts := flattenSelector(v)
		if parts == nil {
			return nil
		}
		name := &ast.DottedName{
			Parts:   parts,
			SrcSpan: token.Span{Start: parts[0].SrcSpan.Start, End: parts[len(parts)-1].SrcSpan.End},
		}
		return &ast.NamedType{Name: name, SrcSpan: name.SrcSpan}
	}
	return nil
}

// flattenSelector turns a nested SelectorExpr chain into a flat list
// of identifiers, when all left-hand sides are identifiers themselves.
// Returns nil if the chain contains non-identifier expressions.
func flattenSelector(s *ast.SelectorExpr) []*ast.Ident {
	switch x := s.X.(type) {
	case *ast.Ident:
		return []*ast.Ident{x, s.Sel}
	case *ast.SelectorExpr:
		inner := flattenSelector(x)
		if inner == nil {
			return nil
		}
		return append(inner, s.Sel)
	}
	return nil
}

// parseCallArgs parses positional and keyword arguments up to RParen.
// Keyword: `name = expr`. Positional: bare `expr`.
func (p *Parser) parseCallArgs() []*ast.CallArg {
	var args []*ast.CallArg
	for p.cur.Kind != token.RParen && p.cur.Kind != token.EOF {
		if p.cur.Kind == token.Comma || p.cur.Kind == token.Semi {
			p.advance()
			continue
		}
		// Keyword arg: ident = expr
		if p.cur.Kind == token.Ident && p.peek.Kind == token.Assign {
			name := p.parseIdent("keyword argument")
			p.advance() // '='
			val := p.parseExpr()
			if name != nil && val != nil {
				args = append(args, &ast.CallArg{Name: name, Value: val})
			}
			continue
		}
		// Positional arg.
		a := p.parseExpr()
		if a == nil {
			break
		}
		args = append(args, &ast.CallArg{Value: a})
	}
	return args
}

// parseBlockContent parses the contents of a struct-lit / step-invocation
// body until the given end token. Handles both `name = value` field inits
// and bare statements (step invocations, let, for, if). Disambiguates:
//
//   - `ident =` → field init
//   - `let/for/if/return` → statement
//   - anything else → expression statement (e.g. `std.pkg { ... }`)
func (p *Parser) parseBlockContent(end token.Kind) ([]*ast.FieldInit, []ast.Stmt) {
	var fields []*ast.FieldInit
	var stmts []ast.Stmt
	for p.cur.Kind != end && p.cur.Kind != token.EOF {
		if p.cur.Kind == token.Comma || p.cur.Kind == token.Semi {
			p.advance()
			continue
		}
		// Statement keywords are always statements.
		if p.cur.Kind == token.Let || p.cur.Kind == token.For ||
			p.cur.Kind == token.If || p.cur.Kind == token.Return {
			s := p.parseStmt()
			if s != nil {
				stmts = append(stmts, s)
			}
			continue
		}
		// `ident = ...` is a field init. `ident.xxx ...` or `ident { ... }`
		// is an expression statement.
		if p.cur.Kind == token.Ident && p.peek.Kind == token.Assign {
			name := p.parseIdent("field name")
			p.expect(token.Assign, "field binding")
			val := p.parseExpr()
			if name != nil && val != nil {
				fields = append(fields, &ast.FieldInit{
					Name:    name,
					Value:   val,
					SrcSpan: token.Span{Start: name.SrcSpan.Start, End: val.Span().End},
				})
			}
			continue
		}
		// Everything else: parse as an expression statement.
		s := p.parseStmt()
		if s != nil {
			stmts = append(stmts, s)
		}
	}
	return fields, stmts
}

// parsePrimary parses a primary expression: literals, identifiers,
// parenthesized expressions, collection literals, control-flow
// expressions, and bare struct/map literals.
func (p *Parser) parsePrimary() ast.Expr {
	tok := p.cur
	switch tok.Kind {
	case token.Int:
		p.advance()
		return &ast.IntLit{
			Raw:     string(p.lex.Source()[tok.Pos:tok.End]),
			SrcSpan: token.Span{Start: tok.Pos, End: tok.End},
		}
	case token.String:
		p.advance()
		return &ast.StringLit{
			Parts: []ast.StringPart{&ast.StringText{
				Raw:     string(p.lex.Source()[tok.Pos:tok.End]),
				SrcSpan: token.Span{Start: tok.Pos, End: tok.End},
			}},
			SrcSpan: token.Span{Start: tok.Pos, End: tok.End},
		}
	case token.StringBeg:
		return p.parseInterpString()
	case token.True:
		p.advance()
		return &ast.BoolLit{Value: true, SrcSpan: token.Span{Start: tok.Pos, End: tok.End}}
	case token.False:
		p.advance()
		return &ast.BoolLit{Value: false, SrcSpan: token.Span{Start: tok.Pos, End: tok.End}}
	case token.None:
		p.advance()
		return &ast.NoneLit{SrcSpan: token.Span{Start: tok.Pos, End: tok.End}}
	case token.Self:
		p.advance()
		return &ast.SelfLit{SrcSpan: token.Span{Start: tok.Pos, End: tok.End}}
	case token.Ident:
		// Parse an Ident; postfix handling will attach selectors etc.
		return p.parseIdent("expression")
	case token.LParen:
		p.advance()
		e := p.parseExpr()
		p.expect(token.RParen, "parenthesized expression")
		return e
	case token.LBrack:
		return p.parseListOrComp()
	case token.LBrace:
		return p.parseBraceExpr()
	case token.If:
		return p.parseIfExpr()
	}
	p.errAt(
		token.Span{Start: tok.Pos, End: tok.End},
		"expected expression, got "+tok.Kind.String(),
	)
	p.advance()
	return nil
}

// parseInterpString assembles an interpolated string literal from
// StringBeg / LInterp / expr / RInterp / StringCont / StringEnd tokens.
func (p *Parser) parseInterpString() ast.Expr {
	start := p.cur.Pos
	var parts []ast.StringPart
	end := p.cur.End

	// First segment is a StringBeg.
	beg := p.advance()
	parts = append(parts, &ast.StringText{
		Raw:     string(p.lex.Source()[beg.Pos:beg.End]),
		SrcSpan: token.Span{Start: beg.Pos, End: beg.End},
	})

	for {
		// Next we expect LInterp then an expression then RInterp.
		if p.cur.Kind != token.LInterp {
			p.errAt(
				token.Span{Start: p.cur.Pos, End: p.cur.End},
				"expected ${ in interpolated string",
			)
			break
		}
		interpStart := p.cur.Pos
		p.advance() // LInterp
		expr := p.parseExpr()
		if p.cur.Kind != token.RInterp {
			p.expect(token.RInterp, "string interpolation")
			break
		}
		interpEnd := p.cur.End
		p.advance()
		if expr != nil {
			parts = append(parts, &ast.StringInterp{
				Expr:    expr,
				SrcSpan: token.Span{Start: interpStart, End: interpEnd},
			})
		}

		// What follows is either StringCont (more interps) or StringEnd.
		switch p.cur.Kind {
		case token.StringCont:
			t := p.advance()
			parts = append(parts, &ast.StringText{
				Raw:     string(p.lex.Source()[t.Pos:t.End]),
				SrcSpan: token.Span{Start: t.Pos, End: t.End},
			})
			// Continue loop: another ${ follows.
			continue
		case token.StringEnd:
			t := p.advance()
			parts = append(parts, &ast.StringText{
				Raw:     string(p.lex.Source()[t.Pos:t.End]),
				SrcSpan: token.Span{Start: t.Pos, End: t.End},
			})
			end = t.End
		default:
			p.errAt(
				token.Span{Start: p.cur.Pos, End: p.cur.End},
				"unterminated interpolated string",
			)
		}
		break
	}

	return &ast.StringLit{
		Parts:   parts,
		SrcSpan: token.Span{Start: start, End: end},
	}
}

// parseListOrComp parses `[...]` which may be a list literal or a
// list comprehension.
func (p *Parser) parseListOrComp() ast.Expr {
	start := p.cur.Pos
	p.advance() // '['
	// Empty list.
	if p.cur.Kind == token.RBrack {
		endTok := p.advance()
		return &ast.ListLit{
			Items:   nil,
			SrcSpan: token.Span{Start: start, End: endTok.End},
		}
	}
	first := p.parseExpr()
	// List comprehension: [expr for var in iter (if cond)?]
	if p.cur.Kind == token.For {
		p.advance()
		v := p.parseIdent("comprehension variable")
		p.expect(token.In, "list comprehension")
		iter := p.parseExpr()
		var cond ast.Expr
		if p.cur.Kind == token.If {
			p.advance()
			cond = p.parseExpr()
		}
		endTok := p.expect(token.RBrack, "list comprehension")
		return &ast.ListComp{
			Expr:    first,
			Var:     v,
			Iter:    iter,
			Cond:    cond,
			SrcSpan: token.Span{Start: start, End: endTok.End},
		}
	}
	// Plain list literal.
	items := []ast.Expr{first}
	for p.cur.Kind != token.RBrack && p.cur.Kind != token.EOF {
		if p.cur.Kind == token.Comma || p.cur.Kind == token.Semi {
			p.advance()
			continue
		}
		x := p.parseExpr()
		if x == nil {
			break
		}
		items = append(items, x)
	}
	endTok := p.expect(token.RBrack, "list literal")
	return &ast.ListLit{
		Items:   items,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}

// parseBraceExpr parses `{...}` at expression position. Distinguishes:
//
//	map literal:    { key : value, ... }   -> uses ':' to separate
//	type literal: { name = value, ... }  -> uses '=' to separate
//	empty:          {}                     -> map literal (convention)
func (p *Parser) parseBraceExpr() ast.Expr {
	start := p.cur.Pos
	p.advance() // '{'
	// Empty {} is an empty map literal by convention (caller can
	// coerce via expected type to struct-style if needed).
	if p.cur.Kind == token.RBrace {
		endTok := p.advance()
		return &ast.MapLit{
			Entries: nil,
			SrcSpan: token.Span{Start: start, End: endTok.End},
		}
	}
	// Skip leading semis (blank lines).
	p.skipSemis()
	// Peek ahead: if we see `ident = ...`, it's a struct literal.
	// Otherwise, parse an expression and look for `:` (map) or
	// more expressions (also map).
	if p.cur.Kind == token.Ident && p.peek.Kind == token.Assign {
		return p.finishStructLit(start, nil)
	}
	// Map literal.
	return p.finishMapLit(start)
}

// finishStructLit finishes parsing a struct literal at the current
// position, which starts with `ident = expr`.
func (p *Parser) finishStructLit(start uint32, typ ast.TypeExpr) ast.Expr {
	fields, body := p.parseBlockContent(token.RBrace)
	endTok := p.expect(token.RBrace, "type literal")
	return &ast.StructLit{
		Type:    typ,
		Fields:  fields,
		Body:    body,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}

// finishMapLit finishes parsing a map literal starting with key : value.
func (p *Parser) finishMapLit(start uint32) ast.Expr {
	var entries []*ast.MapEntry
	for p.cur.Kind != token.RBrace && p.cur.Kind != token.EOF {
		if p.cur.Kind == token.Comma || p.cur.Kind == token.Semi {
			p.advance()
			continue
		}
		k := p.parseExpr()
		if k == nil {
			break
		}
		p.expect(token.Colon, "map entry")
		v := p.parseExpr()
		if v == nil {
			break
		}
		entries = append(entries, &ast.MapEntry{Key: k, Value: v})
	}
	endTok := p.expect(token.RBrace, "map literal")
	return &ast.MapLit{
		Entries: entries,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}

// parseIfExpr parses `if cond { then } else { else_ }` as an expression.
// Statement-position ifs are parsed separately in stmt.go.
func (p *Parser) parseIfExpr() ast.Expr {
	start := p.cur.Pos
	p.advance() // 'if'
	prev := p.inCond
	p.inCond = true
	cond := p.parseExpr()
	p.inCond = prev
	p.expect(token.LBrace, "if expression")
	thenExpr := p.parseExpr()
	p.expect(token.RBrace, "if expression")
	p.expect(token.Else, "if expression")
	p.expect(token.LBrace, "if expression")
	elseExpr := p.parseExpr()
	endTok := p.expect(token.RBrace, "if expression")
	return &ast.IfExpr{
		Cond:    cond,
		Then:    thenExpr,
		Else:    elseExpr,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}
