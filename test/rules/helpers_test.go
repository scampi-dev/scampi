// SPDX-License-Identifier: GPL-3.0-only

package rules

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// repoRoot returns the absolute path of the module root by walking up
// from this file's directory until go.mod is found. Tests use it
// instead of "../.." so they work regardless of the `go test` cwd.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate go.mod above test/rules")
		}
		dir = parent
	}
}
