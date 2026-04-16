// SPDX-License-Identifier: GPL-3.0-only

package parse

import (
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
)

// parseTypeExpr parses a type expression:
//
//	T              — named type (possibly dotted)
//	T[T, T]        — generic type
//	T?             — optional type
//	T[T]?          — combined
func (p *Parser) parseTypeExpr() ast.TypeExpr {
	var t ast.TypeExpr

	// Primary: named or generic type.
	name := p.parseDottedName("type name")
	if name == nil {
		return nil
	}

	start := name.SrcSpan.Start

	if p.cur.Kind == token.LBrack {
		// Generic: Name[Arg, Arg, ...]
		// Generics are only valid on single-ident names (e.g. list[T]).
		if len(name.Parts) != 1 {
			p.errAt(name.SrcSpan, CodeGenericOnDotted, "generic arguments only valid on simple type names")
		}
		p.advance() // '['
		var args []ast.TypeExpr
		for p.cur.Kind != token.RBrack && p.cur.Kind != token.EOF {
			if p.cur.Kind == token.Comma {
				p.advance()
				continue
			}
			arg := p.parseTypeExpr()
			if arg == nil {
				break
			}
			args = append(args, arg)
		}
		endTok := p.expect(token.RBrack, "generic type")
		t = &ast.GenericType{
			Name:    name.Parts[0],
			Args:    args,
			SrcSpan: token.Span{Start: start, End: endTok.End},
		}
	} else {
		t = &ast.NamedType{
			Name:    name,
			SrcSpan: name.SrcSpan,
		}
	}

	// Optional suffix: T?
	if p.cur.Kind == token.Question {
		qTok := p.advance()
		t = &ast.OptionalType{
			Inner:   t,
			SrcSpan: token.Span{Start: start, End: qTok.End},
		}
	}
	return t
}
