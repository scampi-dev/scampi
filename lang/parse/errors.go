// SPDX-License-Identifier: GPL-3.0-only

package parse

import (
	"strings"

	"scampi.dev/scampi/lang/token"
)

// Error is a parser error carrying a span and a human-readable message.
// The parser accumulates errors in a side channel; parsing continues
// so the AST remains usable for LSP recovery.
type Error struct {
	Span token.Span
	Msg  string
	Hint string       // optional fix suggestion
	Got  token.Kind   // actually seen (Illegal for "missing something")
	Want []token.Kind // expected kinds (empty if not applicable)
}

func (e Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Msg)
	if len(e.Want) > 0 {
		b.WriteString(" (expected ")
		for i, w := range e.Want {
			if i > 0 {
				b.WriteString(" or ")
			}
			b.WriteString(w.String())
		}
		if e.Got != 0 {
			b.WriteString(", got ")
			b.WriteString(e.Got.String())
		}
		b.WriteString(")")
	}
	if e.Hint != "" {
		b.WriteString(" — ")
		b.WriteString(e.Hint)
	}
	return b.String()
}

func (e Error) GetSpan() (start, end uint32) { return e.Span.Start, e.Span.End }
