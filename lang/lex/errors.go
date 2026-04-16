// SPDX-License-Identifier: GPL-3.0-only

package lex

import (
	"fmt"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/lang/token"
)

// Error is a lexer error carrying a span into the source and a human
// readable message. The lexer accumulates errors in a side channel;
// Next still returns tokens so the parser can recover.
type Error struct {
	Kind ErrKind
	Span token.Span
	Msg  string
}

func (e Error) Error() string                { return fmt.Sprintf("%s: %s", e.Kind.String(), e.Msg) }
func (e Error) GetSpan() (start, end uint32) { return e.Span.Start, e.Span.End }
func (e Error) GetCode() errs.Code           { return e.Kind.Code() }

// ErrKind identifies a class of lexer error. The kind is stable across
// releases and suitable for diagnostic IDs.
type ErrKind uint8

const (
	ErrInvalidChar ErrKind = iota
	ErrUnterminatedString
	ErrUnterminatedComment
	ErrInvalidEscape
	ErrInvalidNumber
	ErrUnterminatedInterp
)

func (k ErrKind) String() string {
	switch k {
	case ErrInvalidChar:
		return "invalid-char"
	case ErrUnterminatedString:
		return "unterminated-string"
	case ErrUnterminatedComment:
		return "unterminated-comment"
	case ErrInvalidEscape:
		return "invalid-escape"
	case ErrInvalidNumber:
		return "invalid-number"
	case ErrUnterminatedInterp:
		return "unterminated-interp"
	}
	return "unknown"
}
