// SPDX-License-Identifier: GPL-3.0-only

package pkg

import "scampi.dev/scampi/errs"

const (
	CodeInstallFailed         errs.Code = "step.pkg.InstallFailed"
	CodeRemoveFailed          errs.Code = "step.pkg.RemoveFailed"
	CodeCacheUpdateFailed     errs.Code = "step.pkg.CacheUpdateFailed"
	CodeRepoKeyInstallFailed  errs.Code = "step.pkg.RepoKeyInstallFailed"
	CodeRepoConfigFailed      errs.Code = "step.pkg.RepoConfigFailed"
	CodeSuiteDetectionFailed  errs.Code = "step.pkg.SuiteDetectionFailed"
	CodeSourceBackendMismatch errs.Code = "step.pkg.SourceBackendMismatch"
)
