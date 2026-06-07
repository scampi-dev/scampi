// SPDX-License-Identifier: GPL-3.0-only

// Package platform abstracts OS-specific primitives. Per-OS
// implementations live in build-tagged platform_<os>.go files.
package platform

import "context"

type Platform struct {
	Signals   Signaler
	Paths     Paths
	Privilege Privilege
}

type Signaler interface {
	ShutdownContext(parent context.Context) (context.Context, context.CancelFunc)
}

type Paths interface {
	StateDir() (string, error)
}

type Privilege interface {
	IsRoot() bool
}
