// SPDX-License-Identifier: GPL-3.0-only

// Package unarchive implements the builtin unarchive step.
//
// It extracts archives to a target directory, tracking state via a
// checksum marker for idempotent re-runs.
package unarchive
