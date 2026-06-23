// SPDX-License-Identifier: GPL-3.0-only

package svcmgr

// Detect probes which init system is available using the given command runner.
// The runner takes a shell command and returns (exit code, error).
// Returns nil if no supported init system is found.
func Detect(run func(cmd string) (int, error)) Backend {
	if code, err := run("command -v systemctl"); err == nil && code == 0 {
		return backends["systemd"]
	}
	if code, err := run("command -v rc-service"); err == nil && code == 0 {
		return backends["openrc"]
	}
	if code, err := run("command -v launchctl"); err == nil && code == 0 {
		return newLaunchctl(run)
	}
	return nil
}
