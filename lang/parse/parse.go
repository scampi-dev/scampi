// SPDX-License-Identifier: GPL-3.0-only

// Package parse is the scampi-lang parser. It consumes tokens from
// lang/lex and produces an AST defined in lang/ast. The parser is
// recursive descent with one-token lookahead and error recovery.
package parse

import (
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/token"
)

// Parser is a recursive-descent parser over a token stream.
type Parser struct {
	lex  *lex.Lexer
	cur  token.Token
	peek token.Token

	// inCond disables struct-literal detection for `Ident { ... }`
	// while parsing if/for/while conditions, where `{` always starts
	// the block body. Classic Go/Rust disambiguation.
	inCond bool

	errs []Error
}

// New wraps a lexer and primes the lookahead.
func New(l *lex.Lexer) *Parser {
	p := &Parser{lex: l}
	p.cur = l.Next()
	p.peek = l.Next()
	return p
}

// Errors returns parser errors accumulated so far. Lexer errors can be
// retrieved from the underlying Lexer separately.
func (p *Parser) Errors() []Error { return p.errs }

// Parse reads the whole file and returns its AST. Callers should also
// check Errors() and lex.Errors() after.
func (p *Parser) Parse() *ast.File {
	start := p.cur.Pos
	f := &ast.File{Name: p.lex.Name()}

	for p.cur.Kind != token.EOF {
		// Skip stray semicolons between declarations.
		if p.cur.Kind == token.Semi {
			p.advance()
			continue
		}
		// Imports must come before any other declaration/statement.
		if p.cur.Kind == token.Import {
			d := p.parseImport()
			if d != nil {
				f.Imports = append(f.Imports, d)
			}
			continue
		}
		// Top-level: declaration or expression statement.
		if isDeclStart(p.cur.Kind) {
			d := p.parseDecl()
			if d != nil {
				f.Decls = append(f.Decls, d)
			}
			continue
		}
		// Otherwise, treat as a top-level statement (e.g. a step
		// invocation at project root).
		s := p.parseStmt()
		if s != nil {
			f.Stmts = append(f.Stmts, s)
		}
	}
	f.SrcSpan = token.Span{Start: start, End: p.cur.End}
	return f
}

// Token stream helpers
// -----------------------------------------------------------------------------

func (p *Parser) advance() token.Token {
	tok := p.cur
	p.cur = p.peek
	p.peek = p.lex.Next()
	return tok
}

// expect consumes the current token if it matches, otherwise records
// an error. Returns the (original) current token; advance happens
// only on match.
func (p *Parser) expect(k token.Kind, ctx string) token.Token {
	if p.cur.Kind == k {
		return p.advance()
	}
	p.errs = append(p.errs, Error{
		Span: token.Span{Start: p.cur.Pos, End: p.cur.End},
		Msg:  "expected " + k.String() + " in " + ctx,
		Got:  p.cur.Kind,
		Want: []token.Kind{k},
	})
	return p.cur
}

// errAt records a parser error at span.
func (p *Parser) errAt(span token.Span, msg string) {
	p.errs = append(p.errs, Error{Span: span, Msg: msg})
}

// synchronize skips tokens until we land on a likely statement
// boundary: Semi, RBrace, or EOF. Used after a parse error to
// re-anchor the parser.
func (p *Parser) synchronize() {
	for {
		switch p.cur.Kind {
		case token.EOF, token.RBrace:
			return
		case token.Semi:
			p.advance()
			return
		}
		p.advance()
	}
}

// skipSemis consumes any number of consecutive Semi tokens.
func (p *Parser) skipSemis() {
	for p.cur.Kind == token.Semi {
		p.advance()
	}
}

// isDeclStart reports whether a token kind can begin a declaration.
func isDeclStart(k token.Kind) bool {
	switch k {
	case token.Struct, token.Enum, token.Func, token.Step, token.Let:
		return true
	}
	return false
}
