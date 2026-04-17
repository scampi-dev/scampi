// SPDX-License-Identifier: GPL-3.0-only

package rest

import "scampi.dev/scampi/errs"

const (
	CodeRequestError       errs.Code = "rest.RequestError"
	CodeHTTPError          errs.Code = "rest.HTTPError"
	CodeResourceQueryError errs.Code = "rest.ResourceQueryError"
)
