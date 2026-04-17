// SPDX-License-Identifier: GPL-3.0-only

package firewall

import "scampi.dev/scampi/errs"

const (
	CodeBackendNotFound errs.Code = "step.firewall.BackendNotFound"
	CodeRuleCheckFailed errs.Code = "step.firewall.RuleCheckFailed"
	CodeRuleApplyFailed errs.Code = "step.firewall.RuleApplyFailed"
	CodePortOutOfRange  errs.Code = "step.firewall.PortOutOfRange"
	CodeInvalidRange    errs.Code = "step.firewall.InvalidRange"
)
