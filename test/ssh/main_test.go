// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"fmt"
	"os"
	"testing"

	"scampi.dev/scampi/test/harness"
)

func TestMain(m *testing.M) {
	if os.Getenv("SCAMPI_TEST_CONTAINERS") != "" {
		// Containers were explicitly requested: no runtime is an
		// environment error, not a reason to silently skip coverage.
		if err := harness.DockerProbe(); err != nil {
			msg := "SCAMPI_TEST_CONTAINERS is set but no container runtime is available"
			_, _ = fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
			os.Exit(1)
		}
		if err := harness.StartSharedContainer("scampi-test-ssh"); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to start test container: %v\n", err)
			os.Exit(1)
		}
	}

	code := m.Run()

	harness.StopSharedContainer()

	os.Exit(code)
}
