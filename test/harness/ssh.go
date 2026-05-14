// SPDX-License-Identifier: GPL-3.0-only

package harness

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

	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target/ssh"
)

const (
	SSHTestHost = "localhost"
	SSHTestPort = 2222
	SSHTestUser = "testuser"
)

// SharedSSHEnv holds the shared container environment.
// Initialized by TestMain via StartSharedContainer().
var SharedSSHEnv *SSHTestEnv

// SharedComposeFile is the path to docker-compose.yml for the shared container.
var SharedComposeFile string

// SharedKnownHostsPath is the path to the temp known_hosts file.
var SharedKnownHostsPath string

type SSHTestEnv struct {
	Host    string
	Port    int
	User    string
	KeyPath string
}

// StartSharedContainer starts the SSH container once for all tests.
// Called from TestMain.
func StartSharedContainer() error {
	testDir, err := FindTestSSHDirOrErr()
	if err != nil {
		return err
	}

	keyPath := filepath.Join(testDir, "sshd", "testkey")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("test key not found at %s", keyPath)
	}

	SharedComposeFile = filepath.Join(testDir, "sshd", "docker-compose.yml")

	cmd := exec.Command("docker", "compose", "-f", SharedComposeFile, "up", "-d", "--wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	if err := WaitForSSHOrErr(SSHTestHost, SSHTestPort, 30*time.Second); err != nil {
		return err
	}

	knownHostsPath, err := SetupKnownHostsOrErr(SSHTestHost, SSHTestPort)
	if err != nil {
		return err
	}
	SharedKnownHostsPath = knownHostsPath

	SharedSSHEnv = &SSHTestEnv{
		Host:    SSHTestHost,
		Port:    SSHTestPort,
		User:    SSHTestUser,
		KeyPath: keyPath,
	}

	return nil
}

// StopSharedContainer stops the shared container.
// Called from TestMain defer.
func StopSharedContainer() {
	if SharedComposeFile != "" {
		cmd := exec.Command("docker", "compose", "-f", SharedComposeFile, "down", "-v")
		_ = cmd.Run()
	}
	if SharedKnownHostsPath != "" {
		_ = os.Remove(SharedKnownHostsPath)
	}
}

// RecreateContainer tears down the shared container and brings up a fresh one.
// Tests that modify package state call this to get a clean slate instead of
// trying to undo changes with fragile cleanup commands.
func RecreateContainer(t *testing.T) {
	t.Helper()

	if SharedComposeFile == "" {
		t.Fatal("RecreateContainer: no compose file — TestMain did not start a container")
	}

	cmd := exec.Command("docker", "compose", "-f", SharedComposeFile, "up", "-d", "--force-recreate", "--wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("RecreateContainer: docker compose up failed: %v", err)
	}

	if err := WaitForSSHOrErr(SSHTestHost, SSHTestPort, 30*time.Second); err != nil {
		t.Fatalf("RecreateContainer: %v", err)
	}

	knownHostsPath, err := SetupKnownHostsOrErr(SSHTestHost, SSHTestPort)
	if err != nil {
		t.Fatalf("RecreateContainer: %v", err)
	}

	if SharedKnownHostsPath != "" {
		_ = os.Remove(SharedKnownHostsPath)
	}
	SharedKnownHostsPath = knownHostsPath
}

// SetupSSHTestEnv returns the shared SSH environment.
// The container is already running (started by TestMain).
func SetupSSHTestEnv(t *testing.T) (*SSHTestEnv, func()) {
	t.Helper()

	if os.Getenv("SCAMPI_TEST_CONTAINERS") == "" {
		t.Skip("SSH tests disabled (set SCAMPI_TEST_CONTAINERS=1 to enable)")
	}

	if SharedSSHEnv == nil {
		t.Fatal("SharedSSHEnv not initialized - TestMain should have started container")
	}

	return SharedSSHEnv, func() {}
}

func WaitForSSHOrErr(host string, port int, timeout time.Duration) error {
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

func SetupKnownHostsOrErr(host string, port int) (string, error) {
	// ssh-keyscan can race with sshd startup: a TCP port may be open
	// (per WaitForSSHOrErr) before sshd is fully ready to negotiate.
	// Retry a few times with backoff so slow CI runners don't flake.
	const attempts = 5
	var output []byte
	var lastErr error
	for i := range attempts {
		cmd := exec.Command("ssh-keyscan", "-p", fmt.Sprintf("%d", port), host)
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			output = out
			lastErr = nil
			break
		}
		lastErr = err
		if i < attempts-1 {
			time.Sleep(time.Second)
		}
	}
	if lastErr != nil || len(output) == 0 {
		return "", fmt.Errorf("ssh-keyscan failed after %d attempts: %w", attempts, lastErr)
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

func FindTestSSHDirOrErr() (string, error) {
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

func GenerateTempKey(t *testing.T) string {
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

// ConnectSSH creates an SSH target connected to the shared test container.
func ConnectSSH(t *testing.T, env *SSHTestEnv) *ssh.SSHTarget {
	t.Helper()

	src := source.NewMemSource()
	src.Files[env.KeyPath], _ = os.ReadFile(env.KeyPath)

	sshType := ssh.SSH{}
	cfg := &ssh.Config{
		Host:     env.Host,
		Port:     env.Port,
		User:     env.User,
		Key:      env.KeyPath,
		Insecure: true,
	}

	tgt, err := sshType.Create(context.Background(), src, spec.TargetInstance{
		Config: cfg,
		Fields: map[string]spec.FieldSpan{
			"host": {Value: spec.SourceSpan{}},
		},
	})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	return tgt.(*ssh.SSHTarget)
}
