// SPDX-License-Identifier: GPL-3.0-only

package svcmgr

import "fmt"

// launchctl plist search directories, ordered by convention.
var launchctlDirs = []string{
	"/Library/LaunchDaemons",
	"/Library/LaunchAgents",
	"~/Library/LaunchAgents",
}

type launchctlBackend struct {
	domain string // "system" or "gui/<uid>"
}

func newLaunchctl(run func(cmd string) (int, error)) *launchctlBackend {
	domain := "system"
	if code, err := run("test $(id -u) -ne 0"); err == nil && code == 0 {
		// Non-root user — use gui domain.
		if exitCode, e := run("id -u"); e == nil && exitCode == 0 {
			// We can't capture stdout through the run callback (exit code only),
			// so we use the simpler heuristic: non-root = gui/UID and assume
			// launchctl commands work without an explicit domain for user agents.
			domain = "user"
		}
	}
	return &launchctlBackend{domain: domain}
}

func (b *launchctlBackend) Name() string { return "launchctl" }

func (b *launchctlBackend) NeedsRoot() bool { return b.domain == "system" }

func (b *launchctlBackend) CmdIsActive(name string) string {
	return fmt.Sprintf("launchctl list %s 2>/dev/null", ShellQuote(name))
}

func (b *launchctlBackend) CmdIsEnabled(name string) string {
	return fmt.Sprintf("f=$(%s) && test -n \"$f\"", plistFindExpr(name))
}

func (b *launchctlBackend) CmdStart(name string) string {
	return fmt.Sprintf("f=$(%s) && launchctl load -w \"$f\"", plistFindExpr(name))
}

func (b *launchctlBackend) CmdStop(name string) string {
	return fmt.Sprintf("f=$(%s) && launchctl unload \"$f\"", plistFindExpr(name))
}

func (b *launchctlBackend) CmdEnable(name string) string {
	return fmt.Sprintf("f=$(%s) && launchctl load -w \"$f\"", plistFindExpr(name))
}

func (b *launchctlBackend) CmdDisable(name string) string {
	return fmt.Sprintf("f=$(%s) && launchctl unload -w \"$f\"", plistFindExpr(name))
}

func (b *launchctlBackend) CmdDaemonReload() string { return "" }

func (b *launchctlBackend) CmdRestart(name string) string {
	return fmt.Sprintf("f=$(%s) && launchctl unload \"$f\" && launchctl load -w \"$f\"", plistFindExpr(name))
}

func (b *launchctlBackend) CmdReload(_ string) string { return "" }

// plistFindExpr returns a shell snippet that finds <label>.plist in the
// standard directories and prints its path. Evaluates to empty string (and
// non-zero exit) if not found.
func plistFindExpr(name string) string {
	q := ShellQuote(name + ".plist")
	expr := "ls"
	for _, d := range launchctlDirs {
		expr += " " + d + "/" + q
	}
	expr += " 2>/dev/null | head -1"
	return expr
}
