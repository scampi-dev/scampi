// SPDX-License-Identifier: GPL-3.0-only

package parse

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/lang/token"
)

// Diagnostic codes for parser errors. These are stable identifiers
// surfaced to the LSP and (eventually) the error reference docs on
// scampi.dev — do not rename without updating downstream consumers.

// Semantic parse errors
// -----------------------------------------------------------------------------

const (
	CodeMissingModuleDecl  errs.Code = "parse.MissingModuleDecl"
	CodeUnexpectedToken    errs.Code = "parse.UnexpectedToken"
	CodeExpectedExpr       errs.Code = "parse.ExpectedExpr"
	CodeNotAssignable      errs.Code = "parse.NotAssignable"
	CodeGenericOnDotted    errs.Code = "parse.GenericOnDotted"
	CodeUnterminatedInterp errs.Code = "parse.UnterminatedInterp"
)

// Expected-token parse errors — grouped by expected token, not by
// context. The error message carries the context ("in type body",
// "in for-loop"), the code identifies the class of syntax error.
// -----------------------------------------------------------------------------

const (
	CodeExpectedLBrace errs.Code = "parse.ExpectedLBrace"
	CodeExpectedRBrace errs.Code = "parse.ExpectedRBrace"
	CodeExpectedLParen errs.Code = "parse.ExpectedLParen"
	CodeExpectedRParen errs.Code = "parse.ExpectedRParen"
	CodeExpectedRBrack errs.Code = "parse.ExpectedRBrack"
	CodeExpectedAssign errs.Code = "parse.ExpectedAssign"
	CodeExpectedColon  errs.Code = "parse.ExpectedColon"
	CodeExpectedString errs.Code = "parse.ExpectedString"
	CodeExpectedIdent  errs.Code = "parse.ExpectedIdent"
	CodeExpectedIn     errs.Code = "parse.ExpectedIn"
	CodeExpectedElse   errs.Code = "parse.ExpectedElse"
	CodeExpectedAt     errs.Code = "parse.ExpectedAt"
	CodeExpectedToken  errs.Code = "parse.ExpectedToken" // fallback
)

// expectCode maps a token.Kind to the appropriate expected-token code.
func expectCode(k token.Kind) errs.Code {
	switch k {
	case token.LBrace:
		return CodeExpectedLBrace
	case token.RBrace:
		return CodeExpectedRBrace
	case token.LParen:
		return CodeExpectedLParen
	case token.RParen:
		return CodeExpectedRParen
	case token.RBrack:
		return CodeExpectedRBrack
	case token.Assign:
		return CodeExpectedAssign
	case token.Colon:
		return CodeExpectedColon
	case token.String:
		return CodeExpectedString
	case token.Ident:
		return CodeExpectedIdent
	case token.In:
		return CodeExpectedIn
	case token.Else:
		return CodeExpectedElse
	case token.At:
		return CodeExpectedAt
	case token.RInterp:
		return CodeUnterminatedInterp
	}
	return CodeExpectedToken
}
