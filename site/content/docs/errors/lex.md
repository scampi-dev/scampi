---
title: "lex.*"
weight: 1
---

Lex errors mean the source text is malformed at the character level — the
tokenizer couldn't make sense of what it read. These are the earliest errors
in the pipeline.

## lex.InvalidChar

An unexpected character appeared in the source. scampi source files are UTF-8,
but only ASCII characters are valid outside string literals.

## lex.InvalidEscape

An invalid escape sequence appeared inside a string literal. Valid escapes
are `\\`, `\"`, `\n`, `\t`, `\r`.

{{< err >}}
module main
let x = "path\qfoo" // ← \q is not a valid escape
{{< /err >}}

## lex.InvalidNumber

A numeric literal is malformed — digits after a prefix don't match the base,
or a number is immediately followed by an identifier character.

{{< err >}}
module main
let x = 0b102 // ← 2 is not a binary digit
{{< /err >}}

## lex.UnterminatedComment

A block comment `/* ... */` was opened but the file ended before the closing
`*/`.

## lex.UnterminatedInterp

Reserved for future use. String interpolation termination errors are currently
caught by the parser as `parse.UnterminatedInterp`.

## lex.UnterminatedString

A string literal was opened with `"` but never closed — either a newline
appeared before the closing quote, or the file ended.

{{< err >}}
module main
let x = "hello
{{< /err >}}

{{< fix >}}
module main
let x = "hello"
{{< /fix >}}
