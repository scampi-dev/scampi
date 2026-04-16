---
title: "parse.*"
weight: 2
---

Parse errors mean the token stream doesn't form a valid program. The parser
recovered enough to keep going (so the LSP still works), but the file can't be
compiled until these are fixed.

Most parse errors are "expected X, got Y" — a missing brace, a misplaced
keyword, a forgotten equals sign. The error message says what was expected; the
code tells you the category.

## parse.ExpectedAssign

The parser expected `=` — typically in a `let` binding or field assignment.

{{< err >}}
module main
let x 42
{{< /err >}}

{{< fix >}}
module main
let x = 42
{{< /fix >}}

## parse.ExpectedAt

Expected `@` to begin an attribute annotation. This is rare — it typically
means the parser's internal state is confused by a prior syntax error. Fix
earlier errors first.

## parse.ExpectedColon

A `:` was expected — typically in a field type annotation or map literal entry.

{{< err >}}
module main
type Config {
  name string
}
{{< /err >}}

{{< fix >}}
module main
type Config {
  name: string
}
{{< /fix >}}

## parse.ExpectedElse

The parser expected `else` after an `if` expression's closing `}`. If-expressions
(not if-statements) require both branches.

{{< err >}}
module main
let x = if true { 1 }
{{< /err >}}

{{< fix >}}
module main
let x = if true { 1 } else { 0 }
{{< /fix >}}

## parse.ExpectedExpr

The parser expected an expression but found something else — typically a keyword
or delimiter where a value should be.

{{< err >}}
module main
let x = // ← expression missing after =
{{< /err >}}

{{< fix >}}
module main
let x = 42
{{< /fix >}}

## parse.ExpectedIdent

An identifier was expected but something else appeared. Common in declarations
where a name must follow a keyword.

{{< err >}}
module main
let = 42
{{< /err >}}

{{< fix >}}
module main
let x = 42
{{< /fix >}}

The error message says where the identifier was expected: "in module name",
"in field name", "in for-loop variable", etc.

## parse.ExpectedIn

The `in` keyword was expected in a `for` loop or list comprehension.

{{< err >}}
module main
func f() int {
  for x items {
  }
  return 0
}
{{< /err >}}

{{< fix >}}
module main
func f() int {
  for x in items {
  }
  return 0
}
{{< /fix >}}

## parse.ExpectedLBrace

Expected `{` to open a body — type, function, decl, for-loop, if, or else block.

{{< err >}}
module main
func f() int
  return 0
}
{{< /err >}}

{{< fix >}}
module main
func f() int {
  return 0
}
{{< /fix >}}

## parse.ExpectedLParen

Expected `(` to open a parameter list in a function or decl declaration.

{{< err >}}
module main
func double x: int) int {
  return x + x
}
{{< /err >}}

{{< fix >}}
module main
func double(x: int) int {
  return x + x
}
{{< /fix >}}

## parse.ExpectedRBrace

A `{` was opened but never closed. The parser reached a point where `}` was
required and found something else instead.

{{< err >}}
module main
type Server {
  name: string
// ← missing }
{{< /err >}}

{{< fix >}}
module main
type Server {
  name: string
}
{{< /fix >}}

The error message includes context — "in type body", "in for-loop body",
"in block body" — telling you which `{` is unclosed. In large files, match
your braces from the error location upward.

## parse.ExpectedRBrack

Expected `]` to close an index expression, list literal, list comprehension,
or generic type argument list.

{{< err >}}
module main
let xs = [1, 2, 3
{{< /err >}}

{{< fix >}}
module main
let xs = [1, 2, 3]
{{< /fix >}}

## parse.ExpectedRParen

Expected `)` to close a function call, parameter list, or parenthesized
expression.

{{< err >}}
module main
let x = (1 + 2
{{< /err >}}

{{< fix >}}
module main
let x = (1 + 2)
{{< /fix >}}

## parse.ExpectedString

A string literal was expected. Currently only triggered when `import` is not
followed by a quoted path.

{{< err >}}
module main
import posix
{{< /err >}}

{{< fix >}}
module main
import "std/posix"
{{< /fix >}}

## parse.ExpectedToken

The parser expected a specific token that doesn't fall into one of the named
categories above. This is the fallback code — the error message says exactly
what was expected and in what context.

## parse.GenericOnDotted

Generic type arguments (`[T]`) are only valid on simple type names, not
dotted paths.

{{< err >}}
module main
type Foo {
  x: some.module.Type[string]
}
{{< /err >}}

Use the leaf name directly after importing the module.

## parse.MissingModuleDecl

Every `.scampi` file must begin with a `module` declaration.

{{< err >}}
let x = 1
{{< /err >}}

{{< fix >}}
module main
let x = 1
{{< /fix >}}

The module declaration tells the linker which namespace this file belongs to.
Without it, the file can't participate in multi-file modules or be imported.
Entry-point configs use `module main`; library modules use their own name.

## parse.NotAssignable

The left side of an `=` assignment is not something that can be assigned to.
Only index expressions (`x[i]`) and field access (`x.field`) are assignable.

{{< err >}}
module main
func f() int {
  42 = x
  return 0
}
{{< /err >}}

## parse.UnexpectedToken

The parser hit a token that can't start a declaration or statement at the
current position.

{{< err >}}
module main
) // ← nothing expects a closing paren here
{{< /err >}}

This usually means a stray character or a copy-paste artifact. Check the line
the error points to and the line above it — the real problem is often a missing
delimiter on the previous line.

## parse.UnterminatedInterp

A string interpolation `${ ... }` was opened but the string ended before
it was closed.

{{< err >}}
module main
let x = "hello ${name
{{< /err >}}

{{< fix >}}
module main
let x = "hello ${name}"
{{< /fix >}}
