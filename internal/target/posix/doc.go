// SPDX-License-Identifier: GPL-3.0-only

// Package posix provides shared behavior for POSIX target implementations.
//
// Both local and SSH targets execute commands on POSIX systems. This package
// captures the common logic — package management, service management,
// user/group management — parameterized by how commands get executed.
package posix
