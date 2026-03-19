// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Start shared container if SSH tests are enabled
	if os.Getenv("SCAMPI_TEST_CONTAINERS") != "" {
		if err := startSharedContainer(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to start test container: %v\n", err)
			os.Exit(1)
		}
	}

	code := m.Run()

	// os.Exit does not run deferred functions, so clean up explicitly.
	stopSharedContainer()

	os.Exit(code)
}
