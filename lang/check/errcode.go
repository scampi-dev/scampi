// SPDX-License-Identifier: GPL-3.0-only

package check

import "scampi.dev/scampi/errs"

// Diagnostic codes for checker errors. These are stable identifiers
// used by the LSP for code action matching — do not rename them
// without updating the corresponding code action handlers.
const (
	CodeUndefined          errs.Code = "lang.Undefined"
	CodeUnknownModule      errs.Code = "lang.UnknownModule"
	CodeUnknownType        errs.Code = "lang.UnknownType"
	CodeDuplicateImport    errs.Code = "lang.DuplicateImport"
	CodeDuplicateField     errs.Code = "lang.DuplicateField"
	CodeMissingField       errs.Code = "lang.MissingField"
	CodeUnknownField       errs.Code = "lang.UnknownField"
	CodeTooFewArgs         errs.Code = "lang.TooFewArgs"
	CodeTooManyArgs        errs.Code = "lang.TooManyArgs"
	CodeArgTypeMismatch    errs.Code = "lang.ArgTypeMismatch"
	CodeReturnTypeMismatch errs.Code = "lang.ReturnTypeMismatch"
	CodeLetTypeMismatch    errs.Code = "lang.LetTypeMismatch"
	CodeDuplicateLet       errs.Code = "lang.DuplicateLet"
	CodeDuplicateSymbol    errs.Code = "lang.DuplicateSymbol"
	CodeNotAModule         errs.Code = "lang.NotAModule"
	CodeModuleMemberUndef  errs.Code = "lang.ModuleMemberUndef"
	CodeNoField            errs.Code = "lang.NoField"
	CodeNoVariant          errs.Code = "lang.NoVariant"
	CodeCannotAccess       errs.Code = "lang.CannotAccess"
	CodeCannotCall         errs.Code = "lang.CannotCall"
	CodeCannotAdd          errs.Code = "lang.CannotAdd"
	CodeCannotIndex        errs.Code = "lang.CannotIndex"
	CodeOpaqueConstruct    errs.Code = "lang.OpaqueConstruct"
	CodeNotStructOrDecl    errs.Code = "lang.NotStructOrDecl"
	CodeNotBlockType       errs.Code = "lang.NotBlockType"
	CodeIfNotBool          errs.Code = "lang.IfNotBool"
	CodeBoolOpNotBool      errs.Code = "lang.BoolOpNotBool"
	CodeArithNotInt        errs.Code = "lang.ArithNotInt"
	CodeNotOpNotBool       errs.Code = "lang.NotOpNotBool"
	CodeUnaryMinusNotInt   errs.Code = "lang.UnaryMinusNotInt"
	CodeListIndexNotInt    errs.Code = "lang.ListIndexNotInt"
	CodeMapKeyMismatch     errs.Code = "lang.MapKeyMismatch"
	CodeGenericArgCount    errs.Code = "lang.GenericArgCount"
	CodeUnknownGenericType errs.Code = "lang.UnknownGenericType"
	CodeIndeterminateType  errs.Code = "lang.IndeterminateType"
	CodeSelfOutsideStep    errs.Code = "lang.SelfOutsideStep"
	CodeMutationOutside    errs.Code = "lang.MutationOutsideFunc"
	CodeFieldTypeMismatch  errs.Code = "lang.FieldTypeMismatch"
	CodeDeclMissingReturn  errs.Code = "lang.DeclMissingReturnType"
	CodeUnknownFieldType   errs.Code = "lang.UnknownFieldType"
	CodeUnknownAttrField   errs.Code = "lang.UnknownAttrFieldType"
	CodeAttrFieldCarries   errs.Code = "lang.AttrFieldCarriesAttr"
	CodeUnknownAttribute   errs.Code = "lang.UnknownAttribute"
	CodeAttrTooManyDots    errs.Code = "lang.AttrTooManyDots"
	CodeMarkerAttrArgs     errs.Code = "lang.MarkerAttrArgs"
	CodeAttrError          errs.Code = "lang.AttrError"
	CodeAmbiguousUFCS      errs.Code = "lang.AmbiguousUFCS"
	CodeNotAllPathsReturn  errs.Code = "lang.NotAllPathsReturn"
	CodeError              errs.Code = "lang.Error"

	// Eval-time codes — used by lang/eval for runtime errors that
	// don't overlap with checker codes above.
	CodeForInRequiresList errs.Code = "lang.ForInRequiresList"
	CodeCannotEvaluate    errs.Code = "lang.CannotEvaluate"
	CodeCannotFillBlock   errs.Code = "lang.CannotFillBlock"
	CodeEnvVarNotSet      errs.Code = "lang.EnvVarNotSet"
	CodeSecretLookup      errs.Code = "lang.SecretLookup"
	CodeCallError         errs.Code = "lang.CallError"
)
