// SPDX-License-Identifier: GPL-3.0-only

//go:build unix

package platform

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// New returns the Unix Platform implementation (Linux, Darwin, BSD).
func New() Platform {
	return Platform{
		Signals:   unixSignals{},
		Paths:     unixPaths{},
		Privilege: unixPrivilege{},
	}
}

type unixSignals struct{}

func (unixSignals) ShutdownContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

type unixPaths struct{}

// ActionLogDir: root gets /var/lib/scampi/actionlog; everyone else
// gets $XDG_STATE_HOME/scampi/actionlog with the standard XDG
// fallback to $HOME/.local/state.
func (unixPaths) ActionLogDir() (string, error) {
	if os.Geteuid() == 0 {
		return "/var/lib/scampi/actionlog", nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "scampi", "actionlog"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "scampi", "actionlog"), nil
}

type unixPrivilege struct{}

func (unixPrivilege) IsRoot() bool { return os.Geteuid() == 0 }
