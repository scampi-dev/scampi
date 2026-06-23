// SPDX-License-Identifier: GPL-3.0-only

// Package diagnostic defines the diagnostics emission layer of scampi.
//
// It translates execution facts into structured events according to policy.
// Diagnostics are observational and must not be consumed to derive execution
// behavior. This package does not perform execution, planning, or state changes.
package diagnostic
