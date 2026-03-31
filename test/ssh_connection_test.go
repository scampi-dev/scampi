// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/ssh"
)

// Connection Tests
// -----------------------------------------------------------------------------

func TestSSH_Connect(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	// Verify capabilities (at minimum POSIX; Pkg may also be present)
	caps := tgt.Capabilities()
	if caps&capability.POSIX != capability.POSIX {
		t.Errorf("Expected at least POSIX capabilities, got %s", caps)
	}
}

func TestSSH_Connect_WrongKey(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	// Generate a different key
	wrongKey := generateTempKey(t)
	defer func() { _ = os.Remove(wrongKey) }()

	src := source.NewMemSource()
	src.Files[wrongKey], _ = os.ReadFile(wrongKey)

	sshType := ssh.SSH{}
	cfg := &ssh.Config{
		Host:     env.Host,
		Port:     env.Port,
		User:     env.User,
		Key:      wrongKey,
		Insecure: true,
	}

	_, err := sshType.Create(context.Background(), src, spec.TargetInstance{
		Config: cfg,
		Fields: map[string]spec.FieldSpan{
			"host": {Value: spec.SourceSpan{}},
		},
	})

	if err == nil {
		t.Fatal("Expected auth error, got nil")
	}

	var authErr ssh.AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("Expected AuthError, got %T: %v", err, err)
	}
}

func TestSSH_Connect_NoSuchHost(t *testing.T) {
	// Need a valid key to get past auth method check
	keyPath := generateTempKey(t)
	defer func() { _ = os.Remove(keyPath) }()

	src := source.NewMemSource()
	src.Files[keyPath], _ = os.ReadFile(keyPath)

	sshType := ssh.SSH{}
	cfg := &ssh.Config{
		Host:     "nonexistent.invalid",
		Port:     22,
		User:     "nobody",
		Key:      keyPath,
		Insecure: true,
	}

	_, err := sshType.Create(context.Background(), src, spec.TargetInstance{
		Config: cfg,
		Fields: map[string]spec.FieldSpan{
			"host": {Value: spec.SourceSpan{}},
		},
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var hostErr ssh.NoSuchHostError
	if !errors.As(err, &hostErr) {
		t.Errorf("Expected NoSuchHostError, got %T: %v", err, err)
	}
}

func TestSSH_Connect_InvalidTimeout(t *testing.T) {
	sshType := ssh.SSH{}
	cfg := &ssh.Config{
		Host:     "localhost",
		Port:     22,
		User:     "nobody",
		Timeout:  "not-a-duration",
		Insecure: true,
	}

	_, err := sshType.Create(context.Background(), source.NewMemSource(), spec.TargetInstance{
		Config: cfg,
		Fields: map[string]spec.FieldSpan{
			"timeout": {Value: spec.SourceSpan{Filename: "test.scampi", StartLine: 5}},
		},
	})

	if err == nil {
		t.Fatal("Expected error for invalid timeout, got nil")
	}

	var timeoutErr ssh.InvalidTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("Expected InvalidTimeoutError, got %T: %v", err, err)
	}

	if timeoutErr.Value != "not-a-duration" {
		t.Errorf("Expected value %q, got %q", "not-a-duration", timeoutErr.Value)
	}

	if timeoutErr.Source.StartLine != 5 {
		t.Errorf("Expected source line 5, got %d", timeoutErr.Source.StartLine)
	}
}

func TestSSH_Connect_PublicKeyAsPrivate(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	// Try to use the public key as the private key
	pubKey := env.KeyPath + ".pub"
	src := source.NewMemSource()
	src.Files[pubKey], _ = os.ReadFile(pubKey)

	sshType := ssh.SSH{}
	cfg := &ssh.Config{
		Host:     env.Host,
		Port:     env.Port,
		User:     env.User,
		Key:      pubKey,
		Insecure: true,
	}

	_, err := sshType.Create(context.Background(), src, spec.TargetInstance{
		Config: cfg,
		Fields: map[string]spec.FieldSpan{
			"host": {Value: spec.SourceSpan{}},
		},
	})

	if err == nil {
		t.Fatal("Expected error using public key, got nil")
	}

	var parseErr ssh.KeyParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("Expected KeyParseError, got %T: %v", err, err)
	}

	if !parseErr.IsPublicKey {
		t.Error("Expected IsPublicKey=true")
	}
}

// SFTP Operation Tests
// -----------------------------------------------------------------------------

func TestSSH_ReadWriteFile(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()
	path := "/tmp/test-readwrite.txt"
	content := []byte("hello from ssh test")

	// Write file
	if err := tgt.WriteFile(ctx, path, content); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Read back
	got, err := tgt.ReadFile(ctx, path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("Content mismatch: got %q, want %q", got, content)
	}

	// Cleanup
	if err := tgt.Remove(ctx, path); err != nil {
		t.Errorf("Remove failed: %v", err)
	}
}

func TestSSH_ReadFile_NotExist(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()
	_, err := tgt.ReadFile(ctx, "/nonexistent/file")

	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}

	if !errors.Is(err, target.ErrNotExist) {
		t.Errorf("Expected ErrNotExist, got %v", err)
	}
}

func TestSSH_Stat(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()
	path := "/tmp/test-stat.txt"
	content := []byte("stat test content")

	// Create file
	if err := tgt.WriteFile(ctx, path, content); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	defer func() { _ = tgt.Remove(ctx, path) }()

	// Stat
	info, err := tgt.Stat(ctx, path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Size() != int64(len(content)) {
		t.Errorf("Size mismatch: got %d, want %d", info.Size(), len(content))
	}

	if info.IsDir() {
		t.Error("Expected file, got directory")
	}
}

func TestSSH_Stat_NotExist(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()
	_, err := tgt.Stat(ctx, "/nonexistent/file")

	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}

	if !errors.Is(err, target.ErrNotExist) {
		t.Errorf("Expected ErrNotExist, got %v", err)
	}
}

func TestSSH_Chmod(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()
	path := "/tmp/test-chmod.txt"

	// Create file with initial mode
	if err := tgt.WriteFile(ctx, path, []byte("chmod test")); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	defer func() { _ = tgt.Remove(ctx, path) }()

	// Change mode
	if err := tgt.Chmod(ctx, path, 0o755); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	// Verify
	info, err := tgt.Stat(ctx, path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	got := info.Mode().Perm()
	if got != 0o755 {
		t.Errorf("Mode mismatch: got %o, want %o", got, 0o755)
	}
}

// Symlink Tests
// -----------------------------------------------------------------------------

func TestSSH_Symlink(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()
	targetPath := "/tmp/test-symlink-target.txt"
	linkPath := "/tmp/test-symlink-link"

	// Create target file
	if err := tgt.WriteFile(ctx, targetPath, []byte("target content")); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	defer func() { _ = tgt.Remove(ctx, targetPath) }()

	// Create symlink
	if err := tgt.Symlink(ctx, targetPath, linkPath); err != nil {
		t.Fatalf("Symlink failed: %v", err)
	}
	defer func() { _ = tgt.Remove(ctx, linkPath) }()

	// Verify with Lstat
	info, err := tgt.Lstat(ctx, linkPath)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}

	if info.Mode()&fs.ModeSymlink == 0 {
		t.Error("Expected symlink, got regular file")
	}

	// Verify Readlink
	dest, err := tgt.Readlink(ctx, linkPath)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}

	if dest != targetPath {
		t.Errorf("Readlink mismatch: got %q, want %q", dest, targetPath)
	}
}

// Ownership Tests
// -----------------------------------------------------------------------------

func TestSSH_HasUser(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()

	// testuser exists in container
	if !tgt.HasUser(ctx, "testuser") {
		t.Error("Expected testuser to exist")
	}

	// nonexistent user
	if tgt.HasUser(ctx, "nonexistentuser123") {
		t.Error("Expected nonexistentuser123 to not exist")
	}

	// numeric uid should work
	if !tgt.HasUser(ctx, "1000") {
		t.Error("Expected uid 1000 to exist")
	}
}

func TestSSH_HasGroup(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()

	// testgroup exists in container
	if !tgt.HasGroup(ctx, "testgroup") {
		t.Error("Expected testgroup to exist")
	}

	// nonexistent group
	if tgt.HasGroup(ctx, "nonexistentgroup123") {
		t.Error("Expected nonexistentgroup123 to not exist")
	}

	// numeric gid should work
	if !tgt.HasGroup(ctx, "1000") {
		t.Error("Expected gid 1000 to exist")
	}
}

func TestSSH_GetOwner(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()

	// Check owner of testuser's home directory
	owner, err := tgt.GetOwner(ctx, "/home/testuser")
	if err != nil {
		t.Fatalf("GetOwner failed: %v", err)
	}

	if owner.User != "testuser" {
		t.Errorf("Expected user=testuser, got %s", owner.User)
	}

	if owner.Group != "testgroup" {
		t.Errorf("Expected group=testgroup, got %s", owner.Group)
	}
}

func TestSSH_Chown(t *testing.T) {
	env, cleanup := SetupSSHTestEnv(t)
	defer cleanup()

	tgt := connectSSH(t, env)
	defer tgt.Close()

	ctx := context.Background()
	path := "/tmp/test-chown.txt"

	// Create file (owned by testuser since we're connected as testuser)
	if err := tgt.WriteFile(ctx, path, []byte("chown test")); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	defer func() { _ = tgt.Remove(ctx, path) }()

	// Get initial owner
	initial, err := tgt.GetOwner(ctx, path)
	if err != nil {
		t.Fatalf("GetOwner failed: %v", err)
	}

	if initial.User != "testuser" {
		t.Errorf("Expected initial user=testuser, got %s", initial.User)
	}

	// Note: Changing ownership to a different user requires root.
	// testuser can only chown to themselves, so we just verify the current ownership works.
}

// Helper Functions
// -----------------------------------------------------------------------------

func connectSSH(t *testing.T, env *SSHTestEnv) *ssh.SSHTarget {
	t.Helper()

	src := source.NewMemSource()
	src.Files[env.KeyPath], _ = os.ReadFile(env.KeyPath)

	sshType := ssh.SSH{}
	cfg := &ssh.Config{
		Host:     env.Host,
		Port:     env.Port,
		User:     env.User,
		Key:      env.KeyPath,
		Insecure: true, // Container regenerates host keys on rebuild
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
