// SPDX-License-Identifier: GPL-3.0-only

package osutil

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// RunFuzzyFinder pipes choices through a fuzzy finder (e.g. fzf, sk) and
// returns the selected line. Returns "" if the user cancelled.
func RunFuzzyFinder(finder string, choices []string) (string, error) {
	cmd := exec.Command(finder)
	cmd.Stdin = strings.NewReader(strings.Join(choices, "\n"))
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", nil // user cancelled (fzf exits 130, sk exits 1)
		}
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}
