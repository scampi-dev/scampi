// SPDX-License-Identifier: GPL-3.0-only

package unarchive

import "scampi.dev/scampi/errs"

const (
	CodeUnsupportedArchive errs.Code = "step.unarchive.UnsupportedArchive"
	CodeArchiveNotFound    errs.Code = "step.unarchive.ArchiveNotFound"
	CodeExtractionFailed   errs.Code = "step.unarchive.ExtractionFailed"
	CodeArchiveReadEntry   errs.Code = "step.unarchive.ArchiveReadEntry"
	CodeArchiveRead        errs.Code = "step.unarchive.ArchiveRead"
	CodePartialOwnership   errs.Code = "step.unarchive.PartialOwnership"
)
