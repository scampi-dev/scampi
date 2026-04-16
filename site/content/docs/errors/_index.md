---
title: Error Reference
weight: 12
---

Every diagnostic scampi emits carries a stable code like `parse.MissingModuleDecl`
or `lang.MissingField`. These codes appear in the LSP, in `scampi check` output,
and in the engine's error messages.

This reference documents each code: what triggers it, what it looks like, and how
to fix it. Codes are grouped by the pipeline stage that produces them.

| Namespace                           | Stage         |
| ----------------------------------- | ------------- |
| [`lex.*`]({{< relref "lex" >}})     | Tokenization  |
| [`parse.*`]({{< relref "parse" >}}) | Parsing       |
| [`lang.*`]({{< relref "lang" >}})   | Type checking |
| [`link.*`]({{< relref "link" >}})   | Linking       |

## Reading error codes

The namespace tells you *when* in the pipeline the error was caught:

- **`lex`** — the source text is malformed at the character level (unterminated
  strings, invalid escapes). Fix the syntax.
- **`parse`** — the token stream doesn't form a valid program structure (missing
  braces, unexpected tokens). Usually a typo or a misplaced delimiter.
- **`lang`** — the program parses fine but has a type error, scope error, or
  constraint violation. The checker or evaluator caught a semantic problem.
- **`link`** — the config is valid scampi but references something the engine
  doesn't know about. Usually a missing step type or target registration.

Earlier stages catch errors first. If you see `parse.ExpectedRBrace`, don't
look for type errors yet — fix the syntax and the downstream errors usually
disappear.
