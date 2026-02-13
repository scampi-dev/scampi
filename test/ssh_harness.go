// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const (
	sshTestHost = "localhost"
	sshTestPort = 2222
	sshTestUser = "testuser"
)

// sharedSSHEnv holds the shared container environment.
// Initialized by TestMain via startSharedContainer().
var sharedSSHEnv *SSHTestEnv

// sharedComposeFile is the path to docker-compose.yml for the shared container.
var sharedComposeFile string

// sharedKnownHostsPath is the path to the temp known_hosts file.
var sharedKnownHostsPath string

type SSHTestEnv struct {
	Host    string
	Port    int
	User    string
	KeyPath string
}

// startSharedContainer starts the SSH container once for all tests.
// Called from TestMain.
func startSharedContainer() error {
	testDir, err := findTestSSHDirOrErr()
	if err != nil {
		return err
	}

	keyPath := filepath.Join(testDir, "sshd", "testkey")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("test key not found at %s", keyPath)
	}

	sharedComposeFile = filepath.Join(testDir, "sshd", "docker-compose.yml")

	// Start container
	cmd := exec.Command("docker", "compose", "-f", sharedComposeFile, "up", "-d", "--wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for SSH
	if err := waitForSSHOrErr(sshTestHost, sshTestPort, 30*time.Second); err != nil {
		return err
	}

	// Setup known_hosts
	knownHostsPath, err := setupKnownHostsOrErr(sshTestHost, sshTestPort)
	if err != nil {
		return err
	}
	sharedKnownHostsPath = knownHostsPath

	sharedSSHEnv = &SSHTestEnv{
		Host:    sshTestHost,
		Port:    sshTestPort,
		User:    sshTestUser,
		KeyPath: keyPath,
	}

	return nil
}

// stopSharedContainer stops the shared container.
// Called from TestMain defer.
func stopSharedContainer() {
	if sharedComposeFile != "" {
		cmd := exec.Command("docker", "compose", "-f", sharedComposeFile, "down", "-v")
		_ = cmd.Run()
	}
	if sharedKnownHostsPath != "" {
		_ = os.Remove(sharedKnownHostsPath)
	}
}

// RecreateContainer tears down the shared container and brings up a fresh one.
// Tests that modify package state call this to get a clean slate instead of
// trying to undo changes with fragile cleanup commands.
func RecreateContainer(t *testing.T) {
	t.Helper()

	if sharedComposeFile == "" {
		t.Fatal("RecreateContainer: no compose file — TestMain did not start a container")
	}

	cmd := exec.Command("docker", "compose", "-f", sharedComposeFile, "up", "-d", "--force-recreate", "--wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("RecreateContainer: docker compose up failed: %v", err)
	}

	if err := waitForSSHOrErr(sshTestHost, sshTestPort, 30*time.Second); err != nil {
		t.Fatalf("RecreateContainer: %v", err)
	}

	knownHostsPath, err := setupKnownHostsOrErr(sshTestHost, sshTestPort)
	if err != nil {
		t.Fatalf("RecreateContainer: %v", err)
	}

	// Clean up old known_hosts, swap in new one
	if sharedKnownHostsPath != "" {
		_ = os.Remove(sharedKnownHostsPath)
	}
	sharedKnownHostsPath = knownHostsPath
}

// SetupSSHTestEnv returns the shared SSH environment.
// The container is already running (started by TestMain).
func SetupSSHTestEnv(t *testing.T) (*SSHTestEnv, func()) {
	t.Helper()

	if os.Getenv("DOIT_TEST_CONTAINERS") == "" {
		t.Skip("SSH tests disabled (set DOIT_TEST_CONTAINERS=1 to enable)")
	}

	if sharedSSHEnv == nil {
		t.Fatal("sharedSSHEnv not initialized - TestMain should have started container")
	}

	// No per-test cleanup needed - container stays running
	return sharedSSHEnv, func() {}
}

func waitForSSHOrErr(host string, port int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for SSH at %s", addr)
		default:
			conn, err := net.DialTimeout("tcp", addr, time.Second)
			if err == nil {
				_ = conn.Close()
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func setupKnownHostsOrErr(host string, port int) (string, error) {
	cmd := exec.Command("ssh-keyscan", "-p", fmt.Sprintf("%d", port), host)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to scan host key: %w", err)
	}

	f, err := os.CreateTemp("", "known_hosts")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(output); err != nil {
		return "", err
	}

	return f.Name(), nil
}

func findTestSSHDirOrErr() (string, error) {
	dir, _ := os.Getwd()
	for {
		candidate := filepath.Join(dir, "test", "sshd")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "test"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find test/sshd directory")
		}
		dir = parent
	}
}

func generateTempKey(t *testing.T) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-ssh-key-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = tmpFile.Close()

	keyPath := tmpFile.Name()
	_ = os.Remove(keyPath) // ssh-keygen wants to create it

	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Remove(keyPath + ".pub")
	})

	return keyPath
}
