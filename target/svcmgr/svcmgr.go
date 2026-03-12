// SPDX-License-Identifier: GPL-3.0-only

// Package svcmgr provides init system detection and command building.
package svcmgr

import (
	"fmt"
	"strings"
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

// ShellQuote wraps s in single quotes, escaping embedded single quotes.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
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
	return fmt.Sprintf(b.isActive, ShellQuote(name))
}

func (b *templateBackend) CmdIsEnabled(name string) string {
	return fmt.Sprintf(b.isEnabled, ShellQuote(name))
}

func (b *templateBackend) CmdStart(name string) string {
	return fmt.Sprintf(b.start, ShellQuote(name))
}

func (b *templateBackend) CmdStop(name string) string {
	return fmt.Sprintf(b.stop, ShellQuote(name))
}

func (b *templateBackend) CmdEnable(name string) string {
	return fmt.Sprintf(b.enable, ShellQuote(name))
}

func (b *templateBackend) CmdDisable(name string) string {
	return fmt.Sprintf(b.disable, ShellQuote(name))
}

func (b *templateBackend) CmdDaemonReload() string { return b.daemonReload }

func (b *templateBackend) CmdRestart(name string) string {
	return fmt.Sprintf(b.restart, ShellQuote(name))
}

func (b *templateBackend) CmdReload(name string) string {
	if b.reload == "" {
		return ""
	}
	return fmt.Sprintf(b.reload, ShellQuote(name))
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
