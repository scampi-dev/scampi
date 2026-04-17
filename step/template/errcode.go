// SPDX-License-Identifier: GPL-3.0-only

package template

import "scampi.dev/scampi/errs"

const (
	CodeEnvKeyNotInValues errs.Code = "step.template.EnvKeyNotInValues"
	CodeSourceMissing     errs.Code = "step.template.SourceMissing"
	CodeParse             errs.Code = "step.template.Parse"
	CodeExec              errs.Code = "step.template.Exec"
	CodeDestDirMissing    errs.Code = "step.template.DestDirMissing"
)
