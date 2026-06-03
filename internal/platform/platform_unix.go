// SPDX-License-Identifier: GPL-3.0-only

//go:build unix

package platform

import (
	"context"
	"errors"
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
		Locker:    unixLocker{},
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

type unixLocker struct{}

// Acquire opens path (creating it if needed) and tries to take an
// exclusive non-blocking advisory lock via flock(2). If another
// process already holds the lock the returned error wraps
// syscall.EWOULDBLOCK so callers can detect contention.
func (unixLocker) Acquire(path string) (Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	return &unixLock{f: f}, nil
}

type unixLock struct {
	f *os.File
}

func (l *unixLock) Release() error {
	if l.f == nil {
		return nil
	}
	uerr := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	cerr := l.f.Close()
	l.f = nil
	if uerr != nil {
		return uerr
	}
	return cerr
}
