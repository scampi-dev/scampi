// SPDX-License-Identifier: GPL-3.0-only

// Package lex is the scampi-lang lexer. It scans a source byte buffer
// and emits a stream of tokens with byte offsets. Line and column are
// resolved lazily by the token package when needed for diagnostics.
// The lexer performs validation (unterminated strings, bad escapes,
// invalid characters) but does not resolve string escape sequences:
// that happens at AST construction.
package lex

import (
	"scampi.dev/scampi/lang/token"
)

// Lexer is a byte-driven scanner over a single source buffer. It is
// not safe for concurrent use. The caller retains ownership of src.
type Lexer struct {
	src  []byte
	name string

	pos  int        // current byte offset into src
	prev token.Kind // kind of previous non-Semi token (for ASI)

	// interp is a stack of interpolation frames. Each frame tracks the
	// unmatched '{' depth inside an interpolation expression. Empty
	// means we are not currently inside any interpolation.
	interp []interpFrame

	// After scanStringSegment hits ${ and pushes a frame, the caller
	// still needs an LInterp token synchronized with that point.
	// pending holds that token for the following Next() call.
	pending    token.Token
	hasPending bool

	// resumingString is set when RInterp closes an interp frame; the
	// next Next() call resumes scanning the enclosing string segment.
	resumingString bool

	errs []Error
}

type interpFrame struct {
	braces int // unmatched '{' count (starts at 1 for the ${)
}

// New returns a lexer reading from src. name is retained for callers
// who resolve spans to Pos{} later. src must remain valid until the
// last token is consumed.
func New(name string, src []byte) *Lexer {
	return &Lexer{src: src, name: name}
}

// Name returns the filename attached to this lexer.
func (l *Lexer) Name() string { return l.name }

// Source returns the source buffer this lexer is scanning.
func (l *Lexer) Source() []byte { return l.src }

// Errors returns the accumulated lexer errors. Safe to call any time.
func (l *Lexer) Errors() []Error { return l.errs }

// Next returns the next token. When the source is exhausted the lexer
// emits a final Semi (if the preceding token can terminate a statement)
// followed by EOF tokens forever after.
func (l *Lexer) Next() token.Token {
	// Drain a pending LInterp queued after a StringBeg/StringCont.
	if l.hasPending {
		l.hasPending = false
		return l.pending
	}

	// Resume string segment scanning right after an RInterp closed a frame.
	if l.resumingString {
		l.resumingString = false
		return l.scanStringSegment(false /* fresh */)
	}

	// Skip whitespace / comments; emit Semi on ASI-relevant newlines.
	if tok, ok := l.skipWSAndComments(); ok {
		return tok
	}

	if l.pos >= len(l.src) {
		if l.prev.EndsStatement() {
			return l.emit(token.Semi, uint32(l.pos), uint32(l.pos))
		}
		return l.emit(token.EOF, uint32(l.pos), uint32(l.pos))
	}

	c := l.src[l.pos]
	start := uint32(l.pos)

	switch {
	case isIdentStart(c):
		return l.scanIdent(start)
	case isDigit(c):
		return l.scanNumber(start)
	case c == '"':
		return l.enterString()
	}

	if tok, ok := l.scanPunct(start); ok {
		return tok
	}

	// Unknown byte: record an error, advance, emit Illegal.
	l.pos++
	l.addErr(
		ErrInvalidChar,
		token.Span{Start: start, End: uint32(l.pos)},
		"unexpected character",
	)
	return l.emit(token.Illegal, start, uint32(l.pos))
}

// skipWSAndComments advances past spaces, tabs, newlines, line
// comments (`// ...`) and nested block comments (`/* ... */`).
// Returns (tok, true) when a newline triggers ASI; a block comment
// containing a newline is treated as a newline for ASI purposes.
func (l *Lexer) skipWSAndComments() (token.Token, bool) {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch c {
		case ' ', '\t', '\r':
			l.pos++
		case '\n':
			if l.prev.EndsStatement() && len(l.interp) == 0 {
				tok := l.emit(token.Semi, uint32(l.pos), uint32(l.pos+1))
				l.pos++
				return tok, true
			}
			l.pos++
		case '/':
			// Need lookahead to distinguish // line comment, /* block
			// comment, and / division operator. Bare / falls through
			// to scanPunct.
			if l.pos+1 >= len(l.src) {
				return token.Token{}, false
			}
			next := l.src[l.pos+1]
			switch next {
			case '/':
				// Line comment: skip to end-of-line. Don't consume the
				// newline — the next iteration handles it for ASI.
				l.pos += 2
				for l.pos < len(l.src) && l.src[l.pos] != '\n' {
					l.pos++
				}
			case '*':
				// Nested block comment.
				if tok, ok := l.skipBlockComment(); ok {
					return tok, true
				}
			default:
				return token.Token{}, false
			}
		default:
			return token.Token{}, false
		}
	}
	return token.Token{}, false
}

// skipBlockComment consumes a `/* ... */` block comment that starts
// at l.pos. Block comments nest: `/* /* */ */` is one comment, not
// two. If the comment contains a newline and the previous token can
// terminate a statement, returns a Semi token to trigger ASI (as if
// the comment were a newline). Records ErrUnterminatedComment if the
// comment runs off the end of the source.
func (l *Lexer) skipBlockComment() (token.Token, bool) {
	startPos := uint32(l.pos)
	l.pos += 2 // consume opening /*
	depth := 1
	sawNewline := false
	for l.pos < len(l.src) && depth > 0 {
		c := l.src[l.pos]
		if c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			depth++
			l.pos += 2
			continue
		}
		if c == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			depth--
			l.pos += 2
			continue
		}
		if c == '\n' {
			sawNewline = true
		}
		l.pos++
	}
	if depth > 0 {
		l.addErr(
			ErrUnterminatedComment,
			token.Span{Start: startPos, End: uint32(l.pos)},
			"unterminated block comment",
		)
	}
	if sawNewline && l.prev.EndsStatement() && len(l.interp) == 0 {
		end := uint32(l.pos)
		return l.emit(token.Semi, end, end), true
	}
	return token.Token{}, false
}

// emit records and returns a token, updating ASI state.
func (l *Lexer) emit(k token.Kind, start, end uint32) token.Token {
	l.prev = k
	return token.Token{Kind: k, Pos: start, End: end}
}

func (l *Lexer) addErr(k ErrKind, span token.Span, msg string) {
	l.errs = append(l.errs, Error{Kind: k, Span: span, Msg: msg})
}

// scanIdent reads [ident-start][ident-cont]* and resolves keywords.
func (l *Lexer) scanIdent(start uint32) token.Token {
	for l.pos < len(l.src) && isIdentCont(l.src[l.pos]) {
		l.pos++
	}
	kind := token.Lookup(string(l.src[start:l.pos]))
	return l.emit(kind, start, uint32(l.pos))
}

// scanNumber reads an integer literal (decimal, 0x, 0b, 0o).
func (l *Lexer) scanNumber(start uint32) token.Token {
	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) {
		switch l.src[l.pos+1] {
		case 'x', 'X':
			l.pos += 2
			return l.scanDigits(start, isHexDigit, "hexadecimal")
		case 'b', 'B':
			l.pos += 2
			return l.scanDigits(start, isBinDigit, "binary")
		case 'o', 'O':
			l.pos += 2
			return l.scanDigits(start, isOctDigit, "octal")
		}
	}
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
	}
	if l.pos < len(l.src) && isIdentCont(l.src[l.pos]) {
		for l.pos < len(l.src) && isIdentCont(l.src[l.pos]) {
			l.pos++
		}
		l.addErr(
			ErrInvalidNumber,
			token.Span{Start: start, End: uint32(l.pos)},
			"number followed by identifier character",
		)
		return l.emit(token.Illegal, start, uint32(l.pos))
	}
	return l.emit(token.Int, start, uint32(l.pos))
}

func (l *Lexer) scanDigits(start uint32, pred func(byte) bool, what string) token.Token {
	digitStart := l.pos
	for l.pos < len(l.src) && pred(l.src[l.pos]) {
		l.pos++
	}
	if l.pos == digitStart {
		l.addErr(
			ErrInvalidNumber,
			token.Span{Start: start, End: uint32(l.pos)},
			"expected "+what+" digits after prefix",
		)
		return l.emit(token.Illegal, start, uint32(l.pos))
	}
	if l.pos < len(l.src) && isIdentCont(l.src[l.pos]) {
		for l.pos < len(l.src) && isIdentCont(l.src[l.pos]) {
			l.pos++
		}
		l.addErr(
			ErrInvalidNumber,
			token.Span{Start: start, End: uint32(l.pos)},
			"invalid digits in "+what+" literal",
		)
		return l.emit(token.Illegal, start, uint32(l.pos))
	}
	return l.emit(token.Int, start, uint32(l.pos))
}

// scanPunct tries to scan a punctuation token starting at start.
func (l *Lexer) scanPunct(start uint32) (token.Token, bool) {
	c := l.src[l.pos]
	var peek byte
	if l.pos+1 < len(l.src) {
		peek = l.src[l.pos+1]
	}

	// Inside an interpolation, '{' and '}' adjust brace depth. The '}'
	// that balances the outer ${ closes the interpolation and emits
	// RInterp (the frame is popped; next Next() resumes string mode).
	if len(l.interp) > 0 {
		frame := &l.interp[len(l.interp)-1]
		switch c {
		case '{':
			frame.braces++
		case '}':
			if frame.braces == 1 {
				// This } closes the interpolation.
				l.pos++
				l.interp = l.interp[:len(l.interp)-1]
				l.resumingString = true
				return l.emit(token.RInterp, start, uint32(l.pos)), true
			}
			frame.braces--
		}
	}

	switch c {
	case '+':
		l.pos++
		return l.emit(token.Plus, start, uint32(l.pos)), true
	case '-':
		l.pos++
		return l.emit(token.Minus, start, uint32(l.pos)), true
	case '*':
		l.pos++
		return l.emit(token.Star, start, uint32(l.pos)), true
	case '/':
		l.pos++
		return l.emit(token.Slash, start, uint32(l.pos)), true
	case '%':
		l.pos++
		return l.emit(token.Percent, start, uint32(l.pos)), true
	case '=':
		if peek == '=' {
			l.pos += 2
			return l.emit(token.Eq, start, uint32(l.pos)), true
		}
		l.pos++
		return l.emit(token.Assign, start, uint32(l.pos)), true
	case '!':
		if peek == '=' {
			l.pos += 2
			return l.emit(token.Neq, start, uint32(l.pos)), true
		}
		l.pos++
		return l.emit(token.Not, start, uint32(l.pos)), true
	case '<':
		if peek == '=' {
			l.pos += 2
			return l.emit(token.Leq, start, uint32(l.pos)), true
		}
		l.pos++
		return l.emit(token.Lt, start, uint32(l.pos)), true
	case '>':
		if peek == '=' {
			l.pos += 2
			return l.emit(token.Geq, start, uint32(l.pos)), true
		}
		l.pos++
		return l.emit(token.Gt, start, uint32(l.pos)), true
	case '&':
		if peek == '&' {
			l.pos += 2
			return l.emit(token.And, start, uint32(l.pos)), true
		}
		return token.Token{}, false
	case '|':
		if peek == '|' {
			l.pos += 2
			return l.emit(token.Or, start, uint32(l.pos)), true
		}
		return token.Token{}, false
	case ':':
		l.pos++
		return l.emit(token.Colon, start, uint32(l.pos)), true
	case '.':
		l.pos++
		return l.emit(token.Dot, start, uint32(l.pos)), true
	case '?':
		l.pos++
		return l.emit(token.Question, start, uint32(l.pos)), true
	case ',':
		l.pos++
		return l.emit(token.Comma, start, uint32(l.pos)), true
	case '{':
		l.pos++
		return l.emit(token.LBrace, start, uint32(l.pos)), true
	case '}':
		l.pos++
		return l.emit(token.RBrace, start, uint32(l.pos)), true
	case '[':
		l.pos++
		return l.emit(token.LBrack, start, uint32(l.pos)), true
	case ']':
		l.pos++
		return l.emit(token.RBrack, start, uint32(l.pos)), true
	case '(':
		l.pos++
		return l.emit(token.LParen, start, uint32(l.pos)), true
	case ')':
		l.pos++
		return l.emit(token.RParen, start, uint32(l.pos)), true
	}
	return token.Token{}, false
}

// enterString is called on the opening '"' of a string literal.
func (l *Lexer) enterString() token.Token {
	l.pos++ // consume opening "
	return l.scanStringSegment(true /* fresh */)
}

// scanStringSegment scans text from l.pos up to the next ${ or closing ".
// If fresh is true, this is the first segment after an opening quote
// (emits String or StringBeg); otherwise it is a continuation after
// an RInterp (emits StringCont or StringEnd).
//
// The emitted segment token's Pos/End cover ONLY the text content of
// the segment (not the surrounding " or ${ markers).
func (l *Lexer) scanStringSegment(fresh bool) token.Token {
	start := uint32(l.pos)
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch c {
		case '\\':
			if l.pos+1 >= len(l.src) {
				l.addErr(
					ErrInvalidEscape,
					token.Span{Start: uint32(l.pos), End: uint32(l.pos + 1)},
					"trailing backslash in string",
				)
				l.pos++
				continue
			}
			if !isValidEscape(l.src[l.pos+1]) {
				l.addErr(
					ErrInvalidEscape,
					token.Span{Start: uint32(l.pos), End: uint32(l.pos + 2)},
					"invalid escape sequence",
				)
			}
			l.pos += 2
		case '$':
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '{' {
				// End of segment; next two bytes form ${.
				segEnd := uint32(l.pos)
				interpStart := uint32(l.pos)
				l.pos += 2
				interpEnd := uint32(l.pos)
				l.interp = append(l.interp, interpFrame{braces: 1})
				// Queue LInterp for next Next() call.
				l.pending = token.Token{Kind: token.LInterp, Pos: interpStart, End: interpEnd}
				l.hasPending = true
				l.prev = token.LInterp // LInterp doesn't end a statement
				if fresh {
					return l.emit(token.StringBeg, start, segEnd)
				}
				return l.emit(token.StringCont, start, segEnd)
			}
			l.pos++
		case '"':
			segEnd := uint32(l.pos)
			l.pos++ // consume "
			if fresh {
				return l.emit(token.String, start, segEnd)
			}
			return l.emit(token.StringEnd, start, segEnd)
		case '\n':
			// Unterminated single-line string.
			l.addErr(
				ErrUnterminatedString,
				token.Span{Start: start - 1, End: uint32(l.pos)},
				"unterminated string literal (no closing \")",
			)
			if fresh {
				return l.emit(token.String, start, uint32(l.pos))
			}
			return l.emit(token.StringEnd, start, uint32(l.pos))
		default:
			l.pos++
		}
	}
	// Hit EOF inside string.
	l.addErr(
		ErrUnterminatedString,
		token.Span{Start: start - 1, End: uint32(l.pos)},
		"unterminated string literal (reached end of file)",
	)
	if fresh {
		return l.emit(token.String, start, uint32(l.pos))
	}
	return l.emit(token.StringEnd, start, uint32(l.pos))
}

// Character classes
// -----------------------------------------------------------------------------

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

func isDigit(c byte) bool    { return c >= '0' && c <= '9' }
func isHexDigit(c byte) bool { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func isBinDigit(c byte) bool { return c == '0' || c == '1' }
func isOctDigit(c byte) bool { return c >= '0' && c <= '7' }

func isValidEscape(c byte) bool {
	switch c {
	case 'n', 't', 'r', '\\', '"', '$', '0':
		return true
	}
	return false
}
