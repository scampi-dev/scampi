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

func (unixPaths) StateDir() (string, error) {
	if os.Geteuid() == 0 {
		return "/var/lib/scampi", nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "scampi"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "scampi"), nil
}

type unixPrivilege struct{}

func (unixPrivilege) IsRoot() bool { return os.Geteuid() == 0 }
