// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"io/fs"
	"os"
	"testing"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/ssh"
)

// E2EDriver abstracts over different target backends for e2e testing.
// This allows the same test scenarios to run against MemTarget (fast, always available)
// and SSH target (requires container).
type E2EDriver interface {
	// Name returns the driver name for test output
	Name() string

	// Available returns true if this driver can be used
	Available() bool

	// Setup prepares the target with initial state and returns:
	// - the target instance
	// - a TargetInstance wrapper for engine.New
	// - a cleanup function
	Setup(t *testing.T, initial E2EFiles) (target.Target, spec.TargetInstance, func())

	// Verify checks that target matches expected state
	Verify(t *testing.T, expect E2EFiles)

	// ReadFile reads a file from the target (for verification)
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// GetMode gets file mode from the target
	GetMode(ctx context.Context, path string) (fs.FileMode, error)

	// GetSymlink reads symlink target
	GetSymlink(ctx context.Context, path string) (string, error)
}

// MemDriver uses MemTarget for fast, always-available tests
type MemDriver struct {
	tgt *target.MemTarget
}

func NewMemDriver() *MemDriver {
	return &MemDriver{}
}

func (d *MemDriver) Name() string {
	return "mem"
}

func (d *MemDriver) Available() bool {
	return true
}

func (d *MemDriver) Setup(t *testing.T, initial E2EFiles) (target.Target, spec.TargetInstance, func()) {
	t.Helper()

	d.tgt = target.NewMemTarget()

	// Create placeholder to make /tmp exist as a directory
	d.tgt.Files["/tmp/.scampi-placeholder"] = []byte{}

	// Populate initial state
	for path, content := range initial.Files {
		d.tgt.Files[path] = []byte(content)
	}
	for path, permStr := range initial.Perms {
		perm := parsePermOrDie(t, permStr)
		d.tgt.Modes[path] = perm
	}
	for path, owner := range initial.Owners {
		d.tgt.Owners[path] = target.Owner{User: owner.User, Group: owner.Group}
	}
	for path := range initial.Dirs {
		d.tgt.Dirs[path] = 0o755
		d.tgt.Modes[path] = 0o755
		d.tgt.Owners[path] = target.Owner{User: "testuser", Group: "testgroup"}
	}
	for link, linkTarget := range initial.Symlinks {
		d.tgt.Symlinks[link] = linkTarget
	}
	for pkg, installed := range initial.Pkgs {
		d.tgt.Pkgs[pkg] = installed
	}
	for pkg, upgradable := range initial.Upgradable {
		d.tgt.Upgradable[pkg] = upgradable
	}
	for svc, active := range initial.Services {
		d.tgt.Services[svc] = active
	}
	for svc, enabled := range initial.EnabledServices {
		d.tgt.EnabledServices[svc] = enabled
	}
	if len(initial.Commands) > 0 {
		commands := make(map[string]E2ECommandResult, len(initial.Commands))
		for k, v := range initial.Commands {
			commands[k] = v
		}
		d.tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
			r, ok := commands[cmd]
			if !ok {
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}
			res := target.CommandResult{
				ExitCode: r.ExitCode,
				Stdout:   r.Stdout,
				Stderr:   r.Stderr,
			}
			// Promote After to current result, simulating state change
			if r.After != nil {
				commands[cmd] = *r.After
			}
			return res, nil
		}
	}

	ti := mockTargetInstance(d.tgt)

	return d.tgt, ti, func() {} // No cleanup needed for mem target
}

func (d *MemDriver) Verify(t *testing.T, expect E2EFiles) {
	t.Helper()

	// Verify files
	for path, wantContent := range expect.Files {
		got, ok := d.tgt.Files[path]
		if !ok {
			t.Errorf("expected target file %q to exist", path)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("target file %q: got %q, want %q", path, got, wantContent)
		}
	}

	// Verify directories
	for path := range expect.Dirs {
		if _, ok := d.tgt.Dirs[path]; !ok {
			t.Errorf("expected directory %q to exist", path)
		}
	}

	// Verify permissions
	for path, wantPermStr := range expect.Perms {
		wantPerm := parsePermOrDie(t, wantPermStr)
		gotPerm, ok := d.tgt.Modes[path]
		if !ok {
			t.Errorf("expected target file %q to have permissions", path)
			continue
		}
		if gotPerm != wantPerm {
			t.Errorf("target file %q perms: got %o, want %o", path, gotPerm, wantPerm)
		}
	}

	// Verify symlinks
	for link, wantTarget := range expect.Symlinks {
		gotTarget, ok := d.tgt.Symlinks[link]
		if !ok {
			t.Errorf("expected symlink %q to exist", link)
			continue
		}
		if gotTarget != wantTarget {
			t.Errorf("symlink %q: got target %q, want %q", link, gotTarget, wantTarget)
		}
	}

	// Verify packages
	for pkg, wantInstalled := range expect.Pkgs {
		gotInstalled := d.tgt.Pkgs[pkg]
		if gotInstalled != wantInstalled {
			t.Errorf("package %q: got installed=%v, want installed=%v", pkg, gotInstalled, wantInstalled)
		}
	}

	// Verify services
	for svc, wantActive := range expect.Services {
		gotActive := d.tgt.Services[svc]
		if gotActive != wantActive {
			t.Errorf("service %q: got active=%v, want active=%v", svc, gotActive, wantActive)
		}
	}
	for svc, wantEnabled := range expect.EnabledServices {
		gotEnabled := d.tgt.EnabledServices[svc]
		if gotEnabled != wantEnabled {
			t.Errorf("service %q: got enabled=%v, want enabled=%v", svc, gotEnabled, wantEnabled)
		}
	}

	// Verify restart counts
	for svc, wantCount := range expect.Restarts {
		gotCount := d.tgt.Restarts[svc]
		if gotCount != wantCount {
			t.Errorf("service %q: got %d restarts, want %d", svc, gotCount, wantCount)
		}
	}

	// Verify reload counts
	for svc, wantCount := range expect.Reloads {
		gotCount := d.tgt.Reloads[svc]
		if gotCount != wantCount {
			t.Errorf("service %q: got %d reloads, want %d", svc, gotCount, wantCount)
		}
	}
}

func (d *MemDriver) ReadFile(_ context.Context, path string) ([]byte, error) {
	data, ok := d.tgt.Files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return data, nil
}

func (d *MemDriver) GetMode(_ context.Context, path string) (fs.FileMode, error) {
	mode, ok := d.tgt.Modes[path]
	if !ok {
		return 0, fs.ErrNotExist
	}
	return mode, nil
}

func (d *MemDriver) GetSymlink(_ context.Context, path string) (string, error) {
	link, ok := d.tgt.Symlinks[path]
	if !ok {
		return "", fs.ErrNotExist
	}
	return link, nil
}

// SSHDriver uses SSH target connecting to a test container
type SSHDriver struct {
	env     *SSHTestEnv
	tgt     *ssh.SSHTarget
	cleanup func()
}

func NewSSHDriver() *SSHDriver {
	return &SSHDriver{}
}

func (d *SSHDriver) Name() string {
	return "ssh"
}

func (d *SSHDriver) Available() bool {
	return os.Getenv("SCAMPI_TEST_CONTAINERS") != ""
}

func (d *SSHDriver) Setup(t *testing.T, initial E2EFiles) (target.Target, spec.TargetInstance, func()) {
	t.Helper()

	// Get shared SSH environment (container already running via TestMain)
	env, _ := SetupSSHTestEnv(t)
	d.env = env

	// Connect to SSH
	d.tgt = connectSSH(t, env)

	ctx := context.Background()

	// Clean up any leftover files from previous test runs.
	// We clean paths that tests commonly use, plus any from initial state.
	cleanPaths := []string{
		"/tmp/dest.txt", "/tmp/link", "/tmp/link.txt",
		"/tmp/dest-a.txt", "/tmp/dest-b.txt",
		"/tmp/src.txt", "/tmp/target.txt", "/tmp/new-target.txt",
		"/tmp/mydir",
		"/tmp/greeting.txt", "/tmp/config.txt",
	}
	for path := range initial.Files {
		cleanPaths = append(cleanPaths, path)
	}
	for path := range initial.Dirs {
		cleanPaths = append(cleanPaths, path)
	}
	for link := range initial.Symlinks {
		cleanPaths = append(cleanPaths, link)
	}
	for _, p := range cleanPaths {
		_ = d.tgt.Remove(ctx, p)
	}

	// If this fixture touches package state, recreate the container so we
	// start from a clean slate instead of trying to undo previous changes.
	if len(initial.Pkgs) > 0 || len(initial.Upgradable) > 0 {
		RecreateContainer(t)
		d.tgt = connectSSH(t, env)

		// Seed packages that the fixture expects to be pre-installed.
		var toInstall []string
		for pkg, installed := range initial.Pkgs {
			if installed {
				toInstall = append(toInstall, pkg)
			}
		}
		if len(toInstall) > 0 {
			if err := d.tgt.InstallPkgs(ctx, toInstall); err != nil {
				t.Fatalf("failed to seed packages %v: %v", toInstall, err)
			}
		}
	}

	// Populate initial state
	for path, content := range initial.Files {
		if err := d.tgt.WriteFile(ctx, path, []byte(content)); err != nil {
			t.Fatalf("failed to write initial file %q: %v", path, err)
		}
	}
	for path, permStr := range initial.Perms {
		perm := parsePermOrDie(t, permStr)
		if err := d.tgt.Chmod(ctx, path, perm); err != nil {
			t.Fatalf("failed to chmod initial file %q: %v", path, err)
		}
	}
	for path := range initial.Dirs {
		if err := d.tgt.Mkdir(ctx, path, 0o755); err != nil {
			t.Fatalf("failed to mkdir initial dir %q: %v", path, err)
		}
	}
	for link, linkTarget := range initial.Symlinks {
		if err := d.tgt.Symlink(ctx, linkTarget, link); err != nil {
			t.Fatalf("failed to create initial symlink %q: %v", link, err)
		}
	}
	// Note: Chown requires root, skip for initial state

	// Create TargetInstance that wraps the SSH target
	ti := spec.TargetInstance{
		Type:   ssh.SSH{},
		Config: &ssh.Config{},
	}

	cleanup := func() {
		// Just close the connection - container stays running for next test
		d.tgt.Close()
	}

	d.cleanup = cleanup
	return d.tgt, ti, cleanup
}

func (d *SSHDriver) Verify(t *testing.T, expect E2EFiles) {
	t.Helper()
	ctx := context.Background()

	// Verify files
	for path, wantContent := range expect.Files {
		got, err := d.tgt.ReadFile(ctx, path)
		if err != nil {
			t.Errorf("expected target file %q to exist: %v", path, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("target file %q: got %q, want %q", path, got, wantContent)
		}
	}

	// Verify directories
	for path := range expect.Dirs {
		info, err := d.tgt.Stat(ctx, path)
		if err != nil {
			t.Errorf("expected directory %q to exist: %v", path, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a directory, got mode %v", path, info.Mode())
		}
	}

	// Verify permissions
	for path, wantPermStr := range expect.Perms {
		wantPerm := parsePermOrDie(t, wantPermStr)
		info, err := d.tgt.Stat(ctx, path)
		if err != nil {
			t.Errorf("expected target file %q to exist for perm check: %v", path, err)
			continue
		}
		gotPerm := info.Mode().Perm()
		if gotPerm != wantPerm {
			t.Errorf("target file %q perms: got %o, want %o", path, gotPerm, wantPerm)
		}
	}

	// Verify symlinks
	for link, wantTarget := range expect.Symlinks {
		gotTarget, err := d.tgt.Readlink(ctx, link)
		if err != nil {
			t.Errorf("expected symlink %q to exist: %v", link, err)
			continue
		}
		if gotTarget != wantTarget {
			t.Errorf("symlink %q: got target %q, want %q", link, gotTarget, wantTarget)
		}
	}
}

func (d *SSHDriver) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return d.tgt.ReadFile(ctx, path)
}

func (d *SSHDriver) GetMode(ctx context.Context, path string) (fs.FileMode, error) {
	info, err := d.tgt.Stat(ctx, path)
	if err != nil {
		return 0, err
	}
	return info.Mode().Perm(), nil
}

func (d *SSHDriver) GetSymlink(ctx context.Context, path string) (string, error) {
	return d.tgt.Readlink(ctx, path)
}

// AllDrivers returns all available e2e drivers
func AllDrivers(t *testing.T) []E2EDriver {
	t.Helper()

	drivers := []E2EDriver{
		NewMemDriver(),
	}

	sshDriver := NewSSHDriver()
	if sshDriver.Available() {
		drivers = append(drivers, sshDriver)
	}

	return drivers
}
