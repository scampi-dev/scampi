// SPDX-License-Identifier: GPL-3.0-only

package lex

import "scampi.dev/scampi/errs"

// Diagnostic codes for lexer errors. These are stable identifiers
// surfaced to the LSP and (eventually) the error reference docs on
// scampi.dev — do not rename without updating downstream consumers.
const (
	CodeInvalidChar         errs.Code = "lex.InvalidChar"
	CodeUnterminatedString  errs.Code = "lex.UnterminatedString"
	CodeUnterminatedComment errs.Code = "lex.UnterminatedComment"
	CodeInvalidEscape       errs.Code = "lex.InvalidEscape"
	CodeInvalidNumber       errs.Code = "lex.InvalidNumber"
	CodeUnterminatedInterp  errs.Code = "lex.UnterminatedInterp"
)

// Code returns the stable diagnostic code for this error kind.
func (k ErrKind) Code() errs.Code {
	switch k {
	case ErrInvalidChar:
		return CodeInvalidChar
	case ErrUnterminatedString:
		return CodeUnterminatedString
	case ErrUnterminatedComment:
		return CodeUnterminatedComment
	case ErrInvalidEscape:
		return CodeInvalidEscape
	case ErrInvalidNumber:
		return CodeInvalidNumber
	case ErrUnterminatedInterp:
		return CodeUnterminatedInterp
	}
	return "lex.Unknown"
}
