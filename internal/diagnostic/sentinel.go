// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import "scampi.dev/scampi/internal/errs"

// ErrAlreadyRaised signals that a producer has already raised one or
// more diagnostics through the emitter. Callers use this sentinel to
// short-circuit the pipeline without doing their own diagnostic
// extraction or wrapping - the content is on the events, not on the
// error.
//
// bare-error: sentinel; content lives on the emitted events.
var ErrAlreadyRaised = errs.New("diagnostics already raised")
