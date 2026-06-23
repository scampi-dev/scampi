// SPDX-License-Identifier: GPL-3.0-only

package ctrmgr

// Detect probes which container runtime is available using the given command runner.
// The runner takes a shell command and returns (exit code, error).
// Returns nil if no supported runtime is found.
func Detect(run func(cmd string) (int, error)) *Backend {
	for _, name := range []string{"docker", "podman", "nerdctl", "finch"} {
		if code, err := run("command -v " + name); err != nil || code != 0 {
			continue
		}
		needsRoot := true
		if code, err := run(name + " info >/dev/null 2>&1"); err == nil && code == 0 {
			needsRoot = false
		}
		return &Backend{name: name, NeedsRoot: needsRoot}
	}
	return nil
}
