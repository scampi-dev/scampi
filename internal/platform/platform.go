// SPDX-License-Identifier: GPL-3.0-only

// Package platform abstracts OS-specific primitives the engine and
// CLI need: shutdown signals, default state paths, privilege checks.
// Per-OS implementations live in build-tagged platform_<os>.go files;
// callers see only the portable interfaces here.
package platform

import "context"

// Platform groups the OS-specific capabilities scampi needs. Build
// one with New() at startup and pass it (or its fields) into the
// consumers that care.
type Platform struct {
	Signals   Signaler
	Paths     Paths
	Privilege Privilege
	Locker    Locker
}

// Signaler hides per-OS shutdown signal handling. On Unix this wraps
// signal.NotifyContext with SIGINT and SIGTERM; Windows has its own
// console event story (not yet implemented).
type Signaler interface {
	ShutdownContext(parent context.Context) (context.Context, context.CancelFunc)
}

// Paths provides default filesystem locations for scampi state. CLI
// flags (e.g. --action-log) override these.
type Paths interface {
	ActionLogDir() (string, error)
}

// Privilege exposes "am I root?" semantics. Used to pick
// system-install paths over user-mode ones.
type Privilege interface {
	IsRoot() bool
}

// Locker grants exclusive process-level access to a resource keyed
// by file path. Returns an error if the lock is already held by
// another process; this prevents two scampi processes from
// interleaving writes on the same action log dir.
type Locker interface {
	Acquire(path string) (Lock, error)
}

// Lock is the handle returned by Locker.Acquire. Releasing also
// closes the underlying sentinel file. Process death also releases
// the lock automatically (Unix flock semantics).
type Lock interface {
	Release() error
}
