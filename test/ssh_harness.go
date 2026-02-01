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

type SSHTestEnv struct {
	Host    string
	Port    int
	User    string
	KeyPath string
}

// SetupSSHTestEnv starts the SSH container and returns connection details.
// Call the returned cleanup function when done.
func SetupSSHTestEnv(t *testing.T) (*SSHTestEnv, func()) {
	t.Helper()

	// Skip if not running SSH tests
	if os.Getenv("DOIT_TEST_CONTAINERS") == "" {
		t.Skip("SSH tests disabled (set DOIT_TEST_CONTAINERS=1 to enable)")
	}

	// Find test directory
	testDir := findTestSSHDir(t)
	keyPath := filepath.Join(testDir, "sshd", "testkey")

	// Ensure key exists
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatalf("Test key not found at %s - run generate-keys.sh first", keyPath)
	}

	// Start container
	composeFile := filepath.Join(testDir, "sshd", "docker-compose.yml")
	startContainer(t, composeFile)

	// Wait for SSH to be ready
	waitForSSH(t, sshTestHost, sshTestPort)

	// Add host key to known_hosts (for this test only)
	knownHostsPath := setupKnownHosts(t, sshTestHost, sshTestPort)

	env := &SSHTestEnv{
		Host:    sshTestHost,
		Port:    sshTestPort,
		User:    sshTestUser,
		KeyPath: keyPath,
	}

	cleanup := func() {
		cmd := exec.Command("docker", "compose", "-f", composeFile, "down", "-v")
		_ = cmd.Run() // Best effort
		_ = os.Remove(knownHostsPath)
	}

	return env, cleanup
}

func startContainer(t *testing.T, composeFile string) {
	t.Helper()
	cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d", "--wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to start SSH container: %v", err)
	}
}

func waitForSSH(t *testing.T, host string, port int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for SSH at %s", addr)
		default:
			conn, err := net.DialTimeout("tcp", addr, time.Second)
			if err == nil {
				_ = conn.Close()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func setupKnownHosts(t *testing.T, host string, port int) string {
	t.Helper()

	// Scan host key
	cmd := exec.Command("ssh-keyscan", "-p", fmt.Sprintf("%d", port), host)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to scan host key: %v", err)
	}

	// Write to temp known_hosts
	f, err := os.CreateTemp("", "known_hosts")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(output); err != nil {
		t.Fatal(err)
	}

	return f.Name()
}

func findTestSSHDir(t *testing.T) string {
	// Walk up from current dir to find test/sshd
	dir, _ := os.Getwd()
	for {
		candidate := filepath.Join(dir, "test", "sshd")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "test")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find test/sshd directory")
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

	// Clean up the .pub file too
	t.Cleanup(func() {
		_ = os.Remove(keyPath + ".pub")
	})

	return keyPath
}
