// SPDX-License-Identifier: GPL-3.0-only

package e2e

import (
	"fmt"
	"os"
	"testing"

	"scampi.dev/scampi/test/harness"
)

func TestMain(m *testing.M) {
	if os.Getenv("SCAMPI_TEST_CONTAINERS") != "" {
		if err := harness.StartSharedContainer("scampi-test-e2e"); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to start test container: %v\n", err)
			os.Exit(1)
		}
	}

	code := m.Run()

	harness.StopSharedContainer()

	os.Exit(code)
}
