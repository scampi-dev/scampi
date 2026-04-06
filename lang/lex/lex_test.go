// SPDX-License-Identifier: GPL-3.0-only

package lex

import (
	"testing"

	"scampi.dev/scampi/lang/token"
)

type want struct {
	kind token.Kind
	lit  string // raw text in source slice
}

// tokenize runs the lexer to EOF and returns all emitted tokens.
func tokenize(t *testing.T, src string) ([]token.Token, []Error) {
	t.Helper()
	l := New("test.scampi", []byte(src))
	var toks []token.Token
	for {
		tok := l.Next()
		toks = append(toks, tok)
		if tok.Kind == token.EOF {
			break
		}
		if len(toks) > 10000 {
			t.Fatal("lexer produced too many tokens (infinite loop?)")
		}
	}
	return toks, l.Errors()
}

// assertTokens checks that the emitted tokens (excluding EOF) match
// the expected sequence of kind+literal pairs.
func assertTokens(t *testing.T, src string, wants []want) {
	t.Helper()
	toks, errs := tokenize(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// Strip trailing EOF for comparison.
	if toks[len(toks)-1].Kind == token.EOF {
		toks = toks[:len(toks)-1]
	}
	if len(toks) != len(wants) {
		t.Fatalf("got %d tokens, want %d\ngot:  %v\nwant: %v",
			len(toks), len(wants), toks, wants)
	}
	for i, w := range wants {
		got := toks[i]
		if got.Kind != w.kind {
			t.Errorf("token[%d]: got kind=%v, want %v", i, got.Kind, w.kind)
		}
		gotLit := src[got.Pos:got.End]
		if gotLit != w.lit {
			t.Errorf("token[%d]: got lit=%q, want %q", i, gotLit, w.lit)
		}
	}
}

// Idents and keywords
// -----------------------------------------------------------------------------

func TestIdentsAndKeywords(t *testing.T) {
	assertTokens(t, "foo bar baz", []want{
		{token.Ident, "foo"},
		{token.Ident, "bar"},
		{token.Ident, "baz"},
		{token.Semi, ""},
	})
}

func TestKeywords(t *testing.T) {
	assertTokens(t, "import let func step struct enum for in if else return true false none self", []want{
		{token.Import, "import"},
		{token.Let, "let"},
		{token.Func, "func"},
		{token.Step, "step"},
		{token.Struct, "struct"},
		{token.Enum, "enum"},
		{token.For, "for"},
		{token.In, "in"},
		{token.If, "if"},
		{token.Else, "else"},
		{token.Return, "return"},
		{token.True, "true"},
		{token.False, "false"},
		{token.None, "none"},
		{token.Self, "self"},
		{token.Semi, ""},
	})
}

func TestIdentWithUnderscoreAndDigits(t *testing.T) {
	assertTokens(t, "_foo bar_1 x2y", []want{
		{token.Ident, "_foo"},
		{token.Ident, "bar_1"},
		{token.Ident, "x2y"},
		{token.Semi, ""},
	})
}

// Integers
// -----------------------------------------------------------------------------

func TestIntegerLiterals(t *testing.T) {
	assertTokens(t, "0 42 100 0xFF 0b1010 0o755", []want{
		{token.Int, "0"},
		{token.Int, "42"},
		{token.Int, "100"},
		{token.Int, "0xFF"},
		{token.Int, "0b1010"},
		{token.Int, "0o755"},
		{token.Semi, ""},
	})
}

func TestInvalidNumber(t *testing.T) {
	_, errs := tokenize(t, "123abc")
	if len(errs) == 0 {
		t.Fatal("expected error for 123abc")
	}
	if errs[0].Kind != ErrInvalidNumber {
		t.Errorf("got error kind %v, want ErrInvalidNumber", errs[0].Kind)
	}
}

func TestInvalidHex(t *testing.T) {
	_, errs := tokenize(t, "0x")
	if len(errs) == 0 {
		t.Fatal("expected error for 0x")
	}
	if errs[0].Kind != ErrInvalidNumber {
		t.Errorf("got error kind %v, want ErrInvalidNumber", errs[0].Kind)
	}
}

// Operators and delimiters
// -----------------------------------------------------------------------------

func TestOperators(t *testing.T) {
	assertTokens(t, "+ - * / % == != < > <= >= && || ! = : .", []want{
		{token.Plus, "+"},
		{token.Minus, "-"},
		{token.Star, "*"},
		{token.Slash, "/"},
		{token.Percent, "%"},
		{token.Eq, "=="},
		{token.Neq, "!="},
		{token.Lt, "<"},
		{token.Gt, ">"},
		{token.Leq, "<="},
		{token.Geq, ">="},
		{token.And, "&&"},
		{token.Or, "||"},
		{token.Not, "!"},
		{token.Assign, "="},
		{token.Colon, ":"},
		{token.Dot, "."},
	})
}

func TestDelimiters(t *testing.T) {
	assertTokens(t, "{ } [ ] ( ) ,", []want{
		{token.LBrace, "{"},
		{token.RBrace, "}"},
		{token.LBrack, "["},
		{token.RBrack, "]"},
		{token.LParen, "("},
		{token.RParen, ")"},
		{token.Comma, ","},
	})
}

// Strings
// -----------------------------------------------------------------------------

func TestSimpleString(t *testing.T) {
	assertTokens(t, `"hello"`, []want{
		{token.String, "hello"},
		{token.Semi, ""},
	})
}

func TestStringWithEscapes(t *testing.T) {
	assertTokens(t, `"a\nb\tc\\d\"e"`, []want{
		{token.String, `a\nb\tc\\d\"e`},
		{token.Semi, ""},
	})
}

func TestEmptyString(t *testing.T) {
	assertTokens(t, `""`, []want{
		{token.String, ""},
		{token.Semi, ""},
	})
}

func TestUnterminatedString(t *testing.T) {
	_, errs := tokenize(t, `"no end`)
	if len(errs) == 0 {
		t.Fatal("expected error for unterminated string")
	}
	if errs[0].Kind != ErrUnterminatedString {
		t.Errorf("got error kind %v, want ErrUnterminatedString", errs[0].Kind)
	}
}

func TestInvalidEscape(t *testing.T) {
	_, errs := tokenize(t, `"bad\xescape"`)
	if len(errs) == 0 {
		t.Fatal("expected error for invalid escape")
	}
	if errs[0].Kind != ErrInvalidEscape {
		t.Errorf("got error kind %v, want ErrInvalidEscape", errs[0].Kind)
	}
}

// String interpolation
// -----------------------------------------------------------------------------

func TestInterpolation(t *testing.T) {
	assertTokens(t, `"hello ${name} world"`, []want{
		{token.StringBeg, "hello "},
		{token.LInterp, "${"},
		{token.Ident, "name"},
		{token.RInterp, "}"},
		{token.StringEnd, " world"},
		{token.Semi, ""},
	})
}

func TestMultipleInterpolations(t *testing.T) {
	assertTokens(t, `"${a} and ${b}"`, []want{
		{token.StringBeg, ""},
		{token.LInterp, "${"},
		{token.Ident, "a"},
		{token.RInterp, "}"},
		{token.StringCont, " and "},
		{token.LInterp, "${"},
		{token.Ident, "b"},
		{token.RInterp, "}"},
		{token.StringEnd, ""},
		{token.Semi, ""},
	})
}

func TestInterpolationWithExpression(t *testing.T) {
	assertTokens(t, `"${x + y}"`, []want{
		{token.StringBeg, ""},
		{token.LInterp, "${"},
		{token.Ident, "x"},
		{token.Plus, "+"},
		{token.Ident, "y"},
		{token.RInterp, "}"},
		{token.StringEnd, ""},
		{token.Semi, ""},
	})
}

func TestInterpolationWithMapLiteral(t *testing.T) {
	// Ensures inner { } inside ${...} are not confused with interp close.
	assertTokens(t, `"${ {k: v} }"`, []want{
		{token.StringBeg, ""},
		{token.LInterp, "${"},
		{token.LBrace, "{"},
		{token.Ident, "k"},
		{token.Colon, ":"},
		{token.Ident, "v"},
		{token.RBrace, "}"},
		{token.RInterp, "}"},
		{token.StringEnd, ""},
		{token.Semi, ""},
	})
}

// Comments
// -----------------------------------------------------------------------------

func TestLineComment(t *testing.T) {
	assertTokens(t, "foo # this is a comment\nbar", []want{
		{token.Ident, "foo"},
		{token.Semi, "\n"},
		{token.Ident, "bar"},
		{token.Semi, ""},
	})
}

func TestCommentAtEOF(t *testing.T) {
	assertTokens(t, "foo # trailing comment no newline", []want{
		{token.Ident, "foo"},
		{token.Semi, ""},
	})
}

// Whitespace and ASI
// -----------------------------------------------------------------------------

func TestASIAfterIdent(t *testing.T) {
	assertTokens(t, "foo\nbar", []want{
		{token.Ident, "foo"},
		{token.Semi, "\n"},
		{token.Ident, "bar"},
		{token.Semi, ""},
	})
}

func TestASIAfterLiteral(t *testing.T) {
	assertTokens(t, "42\n\"x\"", []want{
		{token.Int, "42"},
		{token.Semi, "\n"},
		{token.String, "x"},
		{token.Semi, ""},
	})
}

func TestNoASIAfterOperator(t *testing.T) {
	assertTokens(t, "1 +\n2", []want{
		{token.Int, "1"},
		{token.Plus, "+"},
		{token.Int, "2"},
		{token.Semi, ""},
	})
}

func TestNoASIAfterComma(t *testing.T) {
	assertTokens(t, "a,\nb", []want{
		{token.Ident, "a"},
		{token.Comma, ","},
		{token.Ident, "b"},
		{token.Semi, ""},
	})
}

func TestNoASIAfterOpenBrace(t *testing.T) {
	assertTokens(t, "foo {\nbar = 1\n}", []want{
		{token.Ident, "foo"},
		{token.LBrace, "{"},
		{token.Ident, "bar"},
		{token.Assign, "="},
		{token.Int, "1"},
		{token.Semi, "\n"},
		{token.RBrace, "}"},
		{token.Semi, ""},
	})
}

func TestMultipleBlankLines(t *testing.T) {
	// Multiple blank lines should collapse into one Semi.
	assertTokens(t, "a\n\n\nb", []want{
		{token.Ident, "a"},
		{token.Semi, "\n"},
		{token.Ident, "b"},
		{token.Semi, ""},
	})
}

// Realistic snippet
// -----------------------------------------------------------------------------

func TestRealSnippet(t *testing.T) {
	src := `import "std"

let x = 42
std.pkg { packages = ["nginx"], state = present }
`
	toks, errs := tokenize(t, src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// Just check that we produced sensible tokens and ended at EOF.
	last := toks[len(toks)-1]
	if last.Kind != token.EOF {
		t.Errorf("last token should be EOF, got %v", last.Kind)
	}
	// Sanity check: first meaningful token is 'import'.
	if toks[0].Kind != token.Import {
		t.Errorf("first token: got %v, want Import", toks[0].Kind)
	}
}
