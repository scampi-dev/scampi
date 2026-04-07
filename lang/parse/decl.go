// SPDX-License-Identifier: GPL-3.0-only

package parse

import (
	"strconv"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
)

// parseModule parses `module <name>` at the top of a file.
func (p *Parser) parseModule() *ast.ModuleDecl {
	start := p.cur.Pos
	p.advance() // 'module'
	name := p.parseIdent("module name")
	if name == nil {
		p.synchronize()
		return nil
	}
	end := name.SrcSpan.End
	if p.cur.Kind == token.Semi {
		p.advance()
	}
	return &ast.ModuleDecl{
		Name:    name,
		SrcSpan: token.Span{Start: start, End: end},
	}
}

// parseImport parses an import declaration:
//
//	import "path"
func (p *Parser) parseImport() *ast.ImportDecl {
	start := p.cur.Pos
	p.advance() // consume 'import'

	if p.cur.Kind != token.String {
		p.expect(token.String, "import")
		p.synchronize()
		return nil
	}
	pathTok := p.advance()
	raw := p.lex.Source()[pathTok.Pos:pathTok.End]
	path, err := strconv.Unquote(`"` + string(raw) + `"`)
	if err != nil {
		// Raw bytes are already the content between quotes (lexer strips them).
		// If that fails, just use the raw content.
		path = string(raw)
	}

	// Consume terminating semi.
	if p.cur.Kind == token.Semi {
		p.advance()
	}

	return &ast.ImportDecl{
		Path:    path,
		SrcSpan: token.Span{Start: start, End: pathTok.End},
	}
}

// parseDecl dispatches on the current token to the right decl parser.
// The caller has already verified isDeclStart(p.cur.Kind).
func (p *Parser) parseDecl() ast.Decl {
	switch p.cur.Kind {
	case token.Type:
		return p.parseTypeDecl()
	case token.Enum:
		return p.parseEnumDecl()
	case token.Func:
		return p.parseFuncDecl()
	case token.Decl:
		return p.parseDeclDecl()
	case token.Let:
		return p.parseLetDecl()
	}
	// Should not reach here given isDeclStart check.
	p.errAt(
		token.Span{Start: p.cur.Pos, End: p.cur.End},
		"unexpected token: "+p.cur.Kind.String(),
	)
	p.advance()
	return nil
}

// parseTypeDecl:
//
//	type Name { field: type = default, ... }
func (p *Parser) parseTypeDecl() *ast.TypeDecl {
	start := p.cur.Pos
	p.advance() // 'type'

	name := p.parseIdent("type name")
	if name == nil {
		p.synchronize()
		return nil
	}

	p.expect(token.LBrace, "type body")
	fields := p.parseFields(token.RBrace)
	endTok := p.expect(token.RBrace, "type body")

	if p.cur.Kind == token.Semi {
		p.advance()
	}

	return &ast.TypeDecl{
		Name:    name,
		Fields:  fields,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}

// parseEnumDecl:
//
//	enum Name { variant, variant, ... }
func (p *Parser) parseEnumDecl() *ast.EnumDecl {
	start := p.cur.Pos
	p.advance() // 'enum'

	name := p.parseIdent("enum name")
	if name == nil {
		p.synchronize()
		return nil
	}

	p.expect(token.LBrace, "enum body")
	var variants []*ast.Ident
	for p.cur.Kind != token.RBrace && p.cur.Kind != token.EOF {
		// Allow semis between variants (from ASI on newlines).
		if p.cur.Kind == token.Semi {
			p.advance()
			continue
		}
		v := p.parseIdent("enum variant")
		if v == nil {
			p.synchronize()
			continue
		}
		variants = append(variants, v)
		// Variants separated by comma, newline (Semi), or both.
		if p.cur.Kind == token.Comma {
			p.advance()
		}
	}
	endTok := p.expect(token.RBrace, "enum body")

	if p.cur.Kind == token.Semi {
		p.advance()
	}

	return &ast.EnumDecl{
		Name:     name,
		Variants: variants,
		SrcSpan:  token.Span{Start: start, End: endTok.End},
	}
}

// parseFuncDecl:
//
//	func Name(params) ReturnType { body }
func (p *Parser) parseFuncDecl() *ast.FuncDecl {
	start := p.cur.Pos
	p.advance() // 'func'

	name := p.parseIdent("function name")
	if name == nil {
		p.synchronize()
		return nil
	}

	p.expect(token.LParen, "function parameters")
	params := p.parseParams(token.RParen)
	p.expect(token.RParen, "function parameters")

	// Return type is required in v0 (no implicit unit return).
	ret := p.parseTypeExpr()

	p.expect(token.LBrace, "function body")
	body := p.parseBlock()
	endTok := p.expect(token.RBrace, "function body")

	if p.cur.Kind == token.Semi {
		p.advance()
	}

	return &ast.FuncDecl{
		Name:    name,
		Params:  params,
		Ret:     ret,
		Body:    body,
		SrcSpan: token.Span{Start: start, End: endTok.End},
	}
}

// parseDeclDecl:
//
//	decl NAME(params) OutputType { body }   // with body
//	decl NAME(params) OutputType            // stub, no body
//
// Name may be dotted (e.g. container.instance, rest.request).
func (p *Parser) parseDeclDecl() *ast.DeclDecl {
	start := p.cur.Pos
	p.advance() // 'decl'

	name := p.parseDottedName("decl name")
	if name == nil {
		p.synchronize()
		return nil
	}

	p.expect(token.LParen, "decl parameters")
	params := p.parseParams(token.RParen)
	p.expect(token.RParen, "decl parameters")

	// Output type: required for builtins/steps that need to declare it,
	// optional for user-defined (defaults to Step). We accept
	// either "no type + body" or "type + [body]". If the next token is
	// LBrace, there's no output type and the body follows.
	var ret ast.TypeExpr
	if p.cur.Kind != token.LBrace && p.cur.Kind != token.Semi && p.cur.Kind != token.EOF {
		ret = p.parseTypeExpr()
	}

	var body *ast.Block
	end := p.cur.End
	if p.cur.Kind == token.LBrace {
		p.advance() // '{'
		body = p.parseBlock()
		endTok := p.expect(token.RBrace, "decl body")
		end = endTok.End
	}

	if p.cur.Kind == token.Semi {
		p.advance()
	}

	return &ast.DeclDecl{
		Name:    name,
		Params:  params,
		Ret:     ret,
		Body:    body,
		SrcSpan: token.Span{Start: start, End: end},
	}
}

// parseLetDecl:
//
//	let NAME = expr
//	let NAME: TYPE = expr
func (p *Parser) parseLetDecl() *ast.LetDecl {
	start := p.cur.Pos
	p.advance() // 'let'

	name := p.parseIdent("let binding name")
	if name == nil {
		p.synchronize()
		return nil
	}

	var typ ast.TypeExpr
	if p.cur.Kind == token.Colon {
		p.advance()
		typ = p.parseTypeExpr()
	}

	p.expect(token.Assign, "let binding")
	value := p.parseExpr()
	if value == nil {
		p.synchronize()
		return nil
	}

	end := value.Span().End
	if p.cur.Kind == token.Semi {
		end = p.cur.End
		p.advance()
	}

	return &ast.LetDecl{
		Name:    name,
		Type:    typ,
		Value:   value,
		SrcSpan: token.Span{Start: start, End: end},
	}
}

// parseFields parses field declarations inside a type body:
//
//	name: type = default
//	name: type
//
// Fields separated by Semi (ASI from newlines) and/or Comma.
// Stops when it sees the given end token.
func (p *Parser) parseFields(end token.Kind) []*ast.Field {
	var fields []*ast.Field
	for p.cur.Kind != end && p.cur.Kind != token.EOF {
		if p.cur.Kind == token.Semi || p.cur.Kind == token.Comma {
			p.advance()
			continue
		}
		f := p.parseField()
		if f == nil {
			p.synchronize()
			continue
		}
		fields = append(fields, f)
	}
	return fields
}

// parseParams parses function/step parameters inside parens:
//
//	name: type, name: type = default, ...
//
// Params separated by Comma and/or Semi (for multi-line param lists).
func (p *Parser) parseParams(end token.Kind) []*ast.Field {
	var params []*ast.Field
	for p.cur.Kind != end && p.cur.Kind != token.EOF {
		if p.cur.Kind == token.Semi || p.cur.Kind == token.Comma {
			p.advance()
			continue
		}
		f := p.parseField()
		if f == nil {
			// Skip ahead to the next comma or the closing token.
			for p.cur.Kind != token.Comma && p.cur.Kind != end && p.cur.Kind != token.EOF {
				p.advance()
			}
			continue
		}
		params = append(params, f)
	}
	return params
}

// parseField parses `name: type` or `name: type = default`.
func (p *Parser) parseField() *ast.Field {
	name := p.parseIdent("field name")
	if name == nil {
		return nil
	}
	p.expect(token.Colon, "field type annotation")
	typ := p.parseTypeExpr()
	if typ == nil {
		return nil
	}
	var def ast.Expr
	end := typ.Span().End
	if p.cur.Kind == token.Assign {
		p.advance()
		def = p.parseExpr()
		if def != nil {
			end = def.Span().End
		}
	}
	return &ast.Field{
		Name:    name,
		Type:    typ,
		Default: def,
		SrcSpan: token.Span{Start: name.SrcSpan.Start, End: end},
	}
}

// parseIdent parses a single identifier, returning nil on mismatch.
func (p *Parser) parseIdent(ctx string) *ast.Ident {
	if p.cur.Kind != token.Ident {
		p.expect(token.Ident, ctx)
		return nil
	}
	tok := p.advance()
	src := p.lex.Source()
	return &ast.Ident{
		Name:    string(src[tok.Pos:tok.End]),
		SrcSpan: token.Span{Start: tok.Pos, End: tok.End},
	}
}

// parseDottedName parses name or name.name.name (all identifiers).
func (p *Parser) parseDottedName(ctx string) *ast.DottedName {
	first := p.parseIdent(ctx)
	if first == nil {
		return nil
	}
	parts := []*ast.Ident{first}
	for p.cur.Kind == token.Dot {
		p.advance()
		next := p.parseIdent("dotted name")
		if next == nil {
			break
		}
		parts = append(parts, next)
	}
	return &ast.DottedName{
		Parts:   parts,
		SrcSpan: token.Span{Start: first.SrcSpan.Start, End: parts[len(parts)-1].SrcSpan.End},
	}
}
