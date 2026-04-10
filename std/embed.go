// SPDX-License-Identifier: GPL-3.0-only

// Package std provides the scampi standard library stubs.
// These are the authoritative type signatures for all builtin
// types, decls, and functions. Embedded at build time.
package std

import "embed"

//go:embed *.scampi */*.scampi
var FS embed.FS
