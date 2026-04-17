// SPDX-License-Identifier: GPL-3.0-only

package linker

import "scampi.dev/scampi/errs"

// Diagnostic codes for linker-specific errors.
const (
	CodeUnresolved             errs.Code = "link.Unresolved"
	CodeAttributeViolation     errs.Code = "linker.AttributeViolation"
	CodeAttributeDeprecated    errs.Code = "linker.AttributeDeprecated"
	CodeNonLiteralAttributeArg errs.Code = "linker.NonLiteralAttributeArg"
	CodeSecretKeyNotFound      errs.Code = "linker.SecretKeyNotFound"
	CodeSecretKeyLookupFailed  errs.Code = "linker.SecretKeyLookupFailed"
)
