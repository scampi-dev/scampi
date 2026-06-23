// SPDX-License-Identifier: GPL-3.0-only

// Package svcmgr provides init system detection and command building.
package svcmgr

import (
	"fmt"

	"scampi.dev/scampi/internal/target"
)

// Backend builds shell commands for a service manager.
type Backend interface {
	Name() string
	NeedsRoot() bool
	CmdIsActive(name string) string
	CmdIsEnabled(name string) string
	CmdStart(name string) string
	CmdStop(name string) string
	CmdEnable(name string) string
	CmdDisable(name string) string
	CmdDaemonReload() string // "" = not applicable
	CmdRestart(name string) string
	CmdReload(name string) string // "" = not supported
}

// templateBackend
// -----------------------------------------------------------------------------

type templateBackend struct {
	name         string
	isActive     string
	isEnabled    string
	start        string
	stop         string
	enable       string
	disable      string
	daemonReload string
	restart      string
	reload       string // "" = not supported
	needsRoot    bool
}

func (b *templateBackend) Name() string    { return b.name }
func (b *templateBackend) NeedsRoot() bool { return b.needsRoot }

func (b *templateBackend) CmdIsActive(name string) string {
	return fmt.Sprintf(b.isActive, target.ShellQuote(name))
}

func (b *templateBackend) CmdIsEnabled(name string) string {
	return fmt.Sprintf(b.isEnabled, target.ShellQuote(name))
}

func (b *templateBackend) CmdStart(name string) string {
	return fmt.Sprintf(b.start, target.ShellQuote(name))
}

func (b *templateBackend) CmdStop(name string) string {
	return fmt.Sprintf(b.stop, target.ShellQuote(name))
}

func (b *templateBackend) CmdEnable(name string) string {
	return fmt.Sprintf(b.enable, target.ShellQuote(name))
}

func (b *templateBackend) CmdDisable(name string) string {
	return fmt.Sprintf(b.disable, target.ShellQuote(name))
}

func (b *templateBackend) CmdDaemonReload() string { return b.daemonReload }

func (b *templateBackend) CmdRestart(name string) string {
	return fmt.Sprintf(b.restart, target.ShellQuote(name))
}

func (b *templateBackend) CmdReload(name string) string {
	if b.reload == "" {
		return ""
	}
	return fmt.Sprintf(b.reload, target.ShellQuote(name))
}

// Built-in template backends
// -----------------------------------------------------------------------------

var backends = map[string]*templateBackend{
	"systemd": {
		name:         "systemd",
		isActive:     "systemctl is-active %s",
		isEnabled:    "systemctl is-enabled %s",
		start:        "systemctl start %s",
		stop:         "systemctl stop %s",
		enable:       "systemctl enable %s",
		disable:      "systemctl disable %s",
		daemonReload: "systemctl daemon-reload",
		restart:      "systemctl restart %s",
		reload:       "systemctl reload %s",
		needsRoot:    true,
	},
	"openrc": {
		name:      "openrc",
		isActive:  "rc-service %s status",
		isEnabled: "rc-update show default | grep -q %s",
		start:     "rc-service %s start",
		stop:      "rc-service %s stop",
		enable:    "rc-update add %s default",
		disable:   "rc-update del %s default",
		restart:   "rc-service %s restart",
		reload:    "rc-service %s reload",
		needsRoot: true,
	},
}
