// SPDX-License-Identifier: GPL-3.0-only

// Package token defines the tokens produced by the scampi lexer
// and consumed by the parser. Tokens carry only byte offsets into the
// source; line and column are resolved lazily from the offsets when
// needed for diagnostics.
package token

// Kind is a token category. Values are stable and assigned explicitly
// so a stringer can be generated deterministically.
type Kind uint8

const (
	Illegal Kind = iota // invalid input, carries the offending bytes
	EOF                 // end of source
	Semi                // statement terminator (newline or ;)

	// Literals
	Ident
	Int
	String     // complete string, no interpolation
	StringBeg  // opening of interpolated string, up to first ${
	StringCont // text segment between two interpolations
	StringEnd  // final text segment of interpolated string

	// Interpolation markers
	LInterp // ${
	RInterp // matching } that closes an interpolation

	// Keywords
	Module
	Import
	Let
	Func
	Decl
	Type
	Enum
	For
	In
	If
	Else
	Return
	True
	False
	None
	Self
	Pub

	// Operators
	Plus     // +
	Minus    // -
	Star     // *
	Slash    // /
	Percent  // %
	Eq       // ==
	Neq      // !=
	Lt       // <
	Gt       // >
	Leq      // <=
	Geq      // >=
	And      // &&
	Or       // ||
	Not      // !
	Assign   // =
	Colon    // :
	Dot      // .
	Question // ?
	At       // @

	// Delimiters
	LBrace // {
	RBrace // }
	LBrack // [
	RBrack // ]
	LParen // (
	RParen // )
	Comma  // ,
)

// Token is a lexed token. Pos and End are byte offsets into the source
// buffer (inclusive start, exclusive end). The token is 12 bytes on
// every architecture Go targets.
type Token struct {
	Kind Kind
	Pos  uint32
	End  uint32
}

// IsLiteral reports whether the token carries a meaningful textual
// value (as opposed to operators and delimiters, which are fully
// identified by their Kind).
func (k Kind) IsLiteral() bool {
	switch k {
	case Ident, Int, String, StringBeg, StringCont, StringEnd:
		return true
	}
	return false
}

// EndsStatement reports whether a token of this kind, when appearing
// at the end of a line, should trigger automatic semicolon insertion.
// Mirrors Go's ASI rules, adapted to scampi.
func (k Kind) EndsStatement() bool {
	switch k {
	case Ident, Int, String, StringEnd,
		True, False, None, Self, Return,
		RBrace, RBrack, RParen:
		return true
	}
	return false
}

// Keywords maps source text to keyword kinds. Identifiers that do not
// match an entry here are returned as Ident.
var Keywords = map[string]Kind{
	"module": Module,
	"import": Import,
	"let":    Let,
	"func":   Func,
	"decl":   Decl,
	"type":   Type,
	"enum":   Enum,
	"for":    For,
	"in":     In,
	"if":     If,
	"else":   Else,
	"return": Return,
	"true":   True,
	"false":  False,
	"none":   None,
	"self":   Self,
	"pub":    Pub,
}

// Lookup returns the keyword kind for s, or Ident if s is not a keyword.
func Lookup(s string) Kind {
	if k, ok := Keywords[s]; ok {
		return k
	}
	return Ident
}

//go:generate stringer -type=Kind
