// SPDX-License-Identifier: GPL-3.0-only

package target

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
)

type commandCall struct {
	Cmd string
}

type MemTarget struct {
	mu sync.RWMutex

	Files      map[string][]byte
	Dirs       map[string]fs.FileMode
	Modes      map[string]fs.FileMode
	Owners     map[string]Owner
	ModTimes   map[string]time.Time
	Symlinks   map[string]string
	Pkgs       map[string]bool
	Upgradable map[string]bool
	CacheStale bool

	Services          map[string]bool // service name -> active (running)
	EnabledServices   map[string]bool // service name -> enabled at boot
	Restarts          map[string]int  // service name -> restart call count
	Reloads           map[string]int  // service name -> reload call count
	ReloadUnsupported bool            // when true, SupportsReload returns false

	Users  map[string]UserInfo  // name -> info
	Groups map[string]GroupInfo // name -> info

	Repos    map[string]RepoConfig // name -> config
	RepoKeys map[string]bool       // name -> installed
	Codename string                // for OSInfoProvider

	Containers map[string]ContainerInfo // name -> info

	Commands    []commandCall
	CommandFunc func(cmd string) (CommandResult, error)
}

func NewMemTarget() *MemTarget {
	return &MemTarget{
		Files:           make(map[string][]byte),
		Dirs:            make(map[string]fs.FileMode),
		Modes:           make(map[string]fs.FileMode),
		Owners:          make(map[string]Owner),
		ModTimes:        make(map[string]time.Time),
		Symlinks:        make(map[string]string),
		Pkgs:            make(map[string]bool),
		Upgradable:      make(map[string]bool),
		Services:        make(map[string]bool),
		EnabledServices: make(map[string]bool),
		Restarts:        make(map[string]int),
		Reloads:         make(map[string]int),
		Users:           make(map[string]UserInfo),
		Groups:          make(map[string]GroupInfo),
		Repos:           make(map[string]RepoConfig),
		RepoKeys:        make(map[string]bool),
		Containers:      make(map[string]ContainerInfo),
	}
}

func (m *MemTarget) ReadFile(_ context.Context, path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.Files[path]
	if !ok {
		return nil, errs.WrapErrf(ErrNotExist, "%q", path)
	}

	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func (m *MemTarget) ReadDir(_ context.Context, path string) ([]fs.DirEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.Dirs[path]; !ok {
		if _, ok := m.Files[path]; !ok {
			return nil, errs.WrapErrf(ErrNotExist, "%q", path)
		}
	}

	prefix := path + "/"
	var entries []fs.DirEntry
	seen := map[string]bool{}

	for p := range m.Files {
		if name, ok := directChild(p, prefix); ok && !seen[name] {
			seen[name] = true
			entries = append(entries, memDirEntry{name: name, dir: false})
		}
	}
	for p := range m.Dirs {
		if name, ok := directChild(p, prefix); ok && !seen[name] {
			seen[name] = true
			entries = append(entries, memDirEntry{name: name, dir: true})
		}
	}

	return entries, nil
}

func (m *MemTarget) WriteFile(_ context.Context, path string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)

	m.Files[path] = cp
	m.Modes[path] = 0o644
	m.ModTimes[path] = time.Now()
	// Set default owner so chown to the same owner is a no-op (matches SSH behavior)
	if _, exists := m.Owners[path]; !exists {
		m.Owners[path] = Owner{User: "testuser", Group: "testgroup"}
	}
	return nil
}

// isImplicitDir reports whether path is a parent of any file or symlink.
// Caller must hold mu.RLock.
func (m *MemTarget) isImplicitDir(path string) bool {
	dirPrefix := path + "/"
	for p := range m.Files {
		if len(p) > len(dirPrefix) && p[:len(dirPrefix)] == dirPrefix {
			return true
		}
	}
	for p := range m.Symlinks {
		if len(p) > len(dirPrefix) && p[:len(dirPrefix)] == dirPrefix {
			return true
		}
	}
	return false
}

func (m *MemTarget) Stat(_ context.Context, path string) (fs.FileInfo, error) {
	if path == "/" {
		return memFileInfo{
			name:  "/",
			mode:  fs.ModeDir | 0o755,
			isDir: true,
		}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.Files[path]
	if ok {
		return memFileInfo{
			name:    path,
			size:    int64(len(data)),
			mode:    m.Modes[path],
			modTime: m.ModTimes[path],
		}, nil
	}

	if dirMode, ok := m.Dirs[path]; ok {
		return memFileInfo{
			name:  path,
			mode:  fs.ModeDir | dirMode,
			isDir: true,
		}, nil
	}

	if m.isImplicitDir(path) {
		return memFileInfo{
			name:  path,
			mode:  fs.ModeDir | 0o755,
			isDir: true,
		}, nil
	}

	return nil, errs.WrapErrf(ErrNotExist, "%q", path)
}

func (m *MemTarget) Mkdir(_ context.Context, path string, mode fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Dirs[path] = mode
	m.Modes[path] = mode
	if _, exists := m.Owners[path]; !exists {
		m.Owners[path] = Owner{User: "testuser", Group: "testgroup"}
	}
	return nil
}

func (m *MemTarget) Chmod(_ context.Context, path string, mode fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, isFile := m.Files[path]
	_, isDir := m.Dirs[path]
	if !isFile && !isDir {
		return errs.WrapErrf(ErrNotExist, "%q", path)
	}

	m.Modes[path] = mode
	m.ModTimes[path] = time.Now()
	if isDir {
		m.Dirs[path] = mode
	}
	return nil
}

func (m *MemTarget) ChmodRecursive(_ context.Context, path string, mode fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := path + "/"
	for p := range m.Files {
		if p == path || (len(p) > len(prefix) && p[:len(prefix)] == prefix) {
			m.Modes[p] = mode
			m.ModTimes[p] = time.Now()
		}
	}
	for p := range m.Dirs {
		if p == path || (len(p) > len(prefix) && p[:len(prefix)] == prefix) {
			m.Modes[p] = mode
			m.Dirs[p] = mode
		}
	}
	return nil
}

func (m *MemTarget) Chown(_ context.Context, path string, owner Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, isFile := m.Files[path]
	_, isDir := m.Dirs[path]
	if !isFile && !isDir {
		return errs.WrapErrf(ErrNotExist, "%q", path)
	}

	m.Owners[path] = owner
	return nil
}

func (m *MemTarget) ChownRecursive(_ context.Context, path string, owner Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := path + "/"
	for p := range m.Files {
		if p == path || (len(p) > len(prefix) && p[:len(prefix)] == prefix) {
			m.Owners[p] = owner
		}
	}
	for p := range m.Dirs {
		if p == path || (len(p) > len(prefix) && p[:len(prefix)] == prefix) {
			m.Owners[p] = owner
		}
	}
	return nil
}

func (m *MemTarget) HasUser(_ context.Context, _ string) bool {
	return true
}

func (m *MemTarget) HasGroup(_ context.Context, _ string) bool {
	return true
}

func (m *MemTarget) GetOwner(_ context.Context, path string) (Owner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, isFile := m.Files[path]
	_, isDir := m.Dirs[path]
	if !isFile && !isDir {
		return Owner{}, errs.WrapErrf(ErrNotExist, "%q", path)
	}

	return m.Owners[path], nil
}

func (m *MemTarget) Lstat(_ context.Context, path string) (fs.FileInfo, error) {
	if path == "/" {
		return memFileInfo{
			name:  "/",
			mode:  fs.ModeDir | 0o755,
			isDir: true,
		}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if target, ok := m.Symlinks[path]; ok {
		return memFileInfo{
			name: path,
			mode: fs.ModeSymlink | 0o777,
			sys:  target,
		}, nil
	}

	data, ok := m.Files[path]
	if ok {
		return memFileInfo{
			name:    path,
			size:    int64(len(data)),
			mode:    m.Modes[path],
			modTime: m.ModTimes[path],
		}, nil
	}

	if m.isImplicitDir(path) {
		return memFileInfo{
			name:  path,
			mode:  fs.ModeDir | 0o755,
			isDir: true,
		}, nil
	}

	return nil, errs.WrapErrf(ErrNotExist, "%q", path)
}

func (m *MemTarget) Readlink(_ context.Context, path string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	target, ok := m.Symlinks[path]
	if !ok {
		return "", errs.WrapErrf(ErrNotExist, "%q", path)
	}

	return target, nil
}

func (m *MemTarget) Symlink(_ context.Context, target, link string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Symlinks[link] = target
	return nil
}

func (m *MemTarget) Remove(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check symlinks first
	if _, ok := m.Symlinks[path]; ok {
		delete(m.Symlinks, path)
		return nil
	}

	// Check directories
	if _, ok := m.Dirs[path]; ok {
		delete(m.Dirs, path)
		return nil
	}

	// Check regular files
	if _, ok := m.Files[path]; ok {
		delete(m.Files, path)
		delete(m.Modes, path)
		delete(m.Owners, path)
		delete(m.ModTimes, path)
		return nil
	}

	return errs.WrapErrf(ErrNotExist, "%q", path)
}

func (m *MemTarget) Capabilities() capability.Capability {
	return capability.POSIX |
		capability.Pkg | capability.PkgUpdate | capability.PkgRepo |
		capability.Service | capability.Container |
		capability.User | capability.Group
}

func (m *MemTarget) IsInstalled(_ context.Context, pkg string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Pkgs[pkg], nil
}

func (m *MemTarget) InstallPkgs(_ context.Context, pkgs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CacheStale {
		return fmt.Errorf("unable to locate package %s", pkgs[0])
	}
	for _, pkg := range pkgs {
		m.Pkgs[pkg] = true
	}
	return nil
}

func (m *MemTarget) RemovePkgs(_ context.Context, pkgs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, pkg := range pkgs {
		delete(m.Pkgs, pkg)
	}
	return nil
}

func (m *MemTarget) UpdateCache(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CacheStale = false
	return nil
}

func (m *MemTarget) CacheAge(_ context.Context) (time.Duration, error) {
	return 0, ErrNoCacheInfo
}

func (m *MemTarget) IsUpgradable(_ context.Context, pkg string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Upgradable[pkg], nil
}

func (m *MemTarget) IsActive(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Services[name], nil
}

func (m *MemTarget) IsEnabled(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.EnabledServices[name], nil
}

func (m *MemTarget) Start(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Services[name] = true
	return nil
}

func (m *MemTarget) Stop(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Services[name] = false
	return nil
}

func (m *MemTarget) Enable(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnabledServices[name] = true
	return nil
}

func (m *MemTarget) Disable(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnabledServices[name] = false
	return nil
}

func (m *MemTarget) Restart(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Restarts[name]++
	return nil
}

func (m *MemTarget) Reload(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Reloads[name]++
	return nil
}

func (m *MemTarget) SupportsReload() bool {
	return !m.ReloadUnsupported
}

func (m *MemTarget) DaemonReload(_ context.Context) error {
	return nil
}

// ContainerManager
// -----------------------------------------------------------------------------

func (m *MemTarget) InspectContainer(_ context.Context, name string) (ContainerInfo, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, ok := m.Containers[name]
	if !ok {
		return ContainerInfo{}, false, nil
	}
	return info, true, nil
}

func (m *MemTarget) CreateContainer(_ context.Context, opts ContainerInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Containers[opts.Name] = ContainerInfo{
		Name:    opts.Name,
		Image:   opts.Image,
		Running: false,
		Restart: opts.Restart,
		Ports:   opts.Ports,
		Env:     opts.Env,
		Mounts:  opts.Mounts,
		Args:    opts.Args,
		Labels:  opts.Labels,
	}
	return nil
}

func (m *MemTarget) StartContainer(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.Containers[name]
	if !ok {
		return errs.WrapErrf(ErrNotExist, "container %q", name)
	}
	info.Running = true
	m.Containers[name] = info
	return nil
}

func (m *MemTarget) StopContainer(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.Containers[name]
	if !ok {
		return nil
	}
	info.Running = false
	m.Containers[name] = info
	return nil
}

func (m *MemTarget) RemoveContainer(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Containers, name)
	return nil
}

func (m *MemTarget) RunCommand(_ context.Context, cmd string) (CommandResult, error) {
	m.mu.Lock()
	m.Commands = append(m.Commands, commandCall{Cmd: cmd})
	fn := m.CommandFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(cmd)
	}
	return CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
}

func (m *MemTarget) RunPrivileged(ctx context.Context, cmd string) (CommandResult, error) {
	return m.RunCommand(ctx, cmd)
}

// UserManager
// -----------------------------------------------------------------------------

func (m *MemTarget) UserExists(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.Users[name]
	return ok, nil
}

func (m *MemTarget) GetUser(_ context.Context, name string) (UserInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.Users[name]
	if !ok {
		return UserInfo{}, errs.WrapErrf(ErrUnknownUser, "%q", name)
	}
	return u, nil
}

func (m *MemTarget) CreateUser(_ context.Context, info UserInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Users[info.Name] = info
	return nil
}

func (m *MemTarget) ModifyUser(_ context.Context, info UserInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Users[info.Name]; !ok {
		return errs.WrapErrf(ErrUnknownUser, "%q", info.Name)
	}
	m.Users[info.Name] = info
	return nil
}

func (m *MemTarget) DeleteUser(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Users, name)
	return nil
}

// GroupManager
// -----------------------------------------------------------------------------

func (m *MemTarget) GroupExists(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.Groups[name]
	return ok, nil
}

func (m *MemTarget) GetGroup(_ context.Context, name string) (GroupInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.Groups[name]
	if !ok {
		return GroupInfo{}, errs.WrapErrf(ErrUnknownGroup, "%q", name)
	}
	return g, nil
}

func (m *MemTarget) CreateGroup(_ context.Context, info GroupInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Groups[info.Name] = info
	return nil
}

func (m *MemTarget) DeleteGroup(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Groups, name)
	return nil
}

// RepoManager
// -----------------------------------------------------------------------------

func (m *MemTarget) HasRepo(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.Repos[name]
	return ok, nil
}

func (m *MemTarget) HasRepoKey(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.RepoKeys[name], nil
}

func (m *MemTarget) InstallRepoKey(_ context.Context, cfg RepoConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RepoKeys[cfg.Name] = true
	return nil
}

func (m *MemTarget) WriteRepoConfig(_ context.Context, cfg RepoConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Repos[cfg.Name] = cfg
	return nil
}

func (m *MemTarget) RemoveRepo(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Repos, name)
	delete(m.RepoKeys, name)
	return nil
}

func (m *MemTarget) RepoKeyPath(name string) string {
	return "/usr/share/keyrings/" + name + ".gpg"
}

func (m *MemTarget) RepoConfigPath(name string) string {
	return "/etc/apt/sources.list.d/scampi-" + name + ".sources"
}

// OSInfoProvider
// -----------------------------------------------------------------------------

func (m *MemTarget) VersionCodename() string {
	return m.Codename
}

type memFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
	sys     any
}

func (i memFileInfo) Name() string       { return i.name }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() fs.FileMode  { return i.mode }
func (i memFileInfo) ModTime() time.Time { return i.modTime }
func (i memFileInfo) IsDir() bool        { return i.isDir }
func (i memFileInfo) Sys() any           { return i.sys }

type memDirEntry struct {
	name string
	dir  bool
}

func (e memDirEntry) Name() string               { return e.name }
func (e memDirEntry) IsDir() bool                { return e.dir }
func (e memDirEntry) Type() fs.FileMode          { return 0 }
func (e memDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func directChild(path, prefix string) (string, bool) {
	if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
		return "", false
	}
	rest := path[len(prefix):]
	if i := strings.Index(rest, "/"); i >= 0 {
		return "", false
	}
	return rest, true
}
