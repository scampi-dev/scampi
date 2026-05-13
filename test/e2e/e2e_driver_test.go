// SPDX-License-Identifier: GPL-3.0-only

package e2e

import (
	"sort"
	"testing"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/test/harness"
)

// setupMemTarget builds a fresh MemTarget seeded from the scenario's
// initial state (the `target.json` payload — files, dirs, perms,
// services, packages, etc.).
//
// The /tmp placeholder file forces /tmp to exist as a directory; some
// step implementations write through /tmp during apply.
func setupMemTarget(t *testing.T, initial E2EFiles) (*target.MemTarget, spec.TargetInstance) {
	t.Helper()

	tgt := target.NewMemTarget()
	tgt.Files["/tmp/.scampi-placeholder"] = []byte{}

	for path, content := range initial.Files {
		tgt.Files[path] = []byte(content)
	}
	for path, permStr := range initial.Perms {
		tgt.Modes[path] = parsePermOrDie(t, permStr)
	}
	for path, owner := range initial.Owners {
		tgt.Owners[path] = target.Owner{User: owner.User, Group: owner.Group}
	}
	for path := range initial.Dirs {
		tgt.Dirs[path] = 0o755
		tgt.Modes[path] = 0o755
		tgt.Owners[path] = target.Owner{User: "testuser", Group: "testgroup"}
	}
	for link, linkTarget := range initial.Symlinks {
		tgt.Symlinks[link] = linkTarget
	}
	for pkg, installed := range initial.Pkgs {
		tgt.Pkgs[pkg] = installed
	}
	for pkg, upgradable := range initial.Upgradable {
		tgt.Upgradable[pkg] = upgradable
	}
	tgt.CacheStale = initial.CacheStale
	for svc, active := range initial.Services {
		tgt.Services[svc] = active
	}
	for svc, enabled := range initial.EnabledServices {
		tgt.EnabledServices[svc] = enabled
	}
	if len(initial.Commands) > 0 {
		commands := make(map[string]E2ECommandResult, len(initial.Commands))
		for k, v := range initial.Commands {
			commands[k] = v
		}
		tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
			r, ok := commands[cmd]
			if !ok {
				return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
			}
			res := target.CommandResult{
				ExitCode: r.ExitCode,
				Stdout:   r.Stdout,
				Stderr:   r.Stderr,
			}
			if r.After != nil {
				commands[cmd] = *r.After
			}
			return res, nil
		}
	}

	for name, installed := range initial.Repos {
		if installed {
			tgt.Repos[name] = target.RepoConfig{Name: name}
		}
	}
	for name, installed := range initial.RepoKeys {
		tgt.RepoKeys[name] = installed
	}
	if initial.VersionCodename != "" {
		tgt.Codename = initial.VersionCodename
	}

	for name, info := range initial.Users {
		tgt.Users[name] = target.UserInfo{
			Name:     name,
			Shell:    info.Shell,
			Home:     info.Home,
			System:   info.System,
			Password: info.Password,
			Groups:   info.Groups,
		}
	}
	for name, info := range initial.Groups {
		tgt.Groups[name] = target.GroupInfo{
			Name:   name,
			GID:    info.GID,
			System: info.System,
		}
	}

	return tgt, harness.MockTargetInstance(tgt)
}

// verifyMemTarget asserts that the post-Apply MemTarget state matches
// the scenario's expectations (the `expect.json` target block — files,
// perms, services, packages, etc.).
func verifyMemTarget(t *testing.T, tgt *target.MemTarget, expect E2EFiles) {
	t.Helper()

	for path, wantContent := range expect.Files {
		got, ok := tgt.Files[path]
		if !ok {
			t.Errorf("expected target file %q to exist", path)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("target file %q: got %q, want %q", path, got, wantContent)
		}
	}

	for path := range expect.Dirs {
		if _, ok := tgt.Dirs[path]; !ok {
			t.Errorf("expected directory %q to exist", path)
		}
	}

	for path, want := range expect.Owners {
		got, ok := tgt.Owners[path]
		if !ok {
			t.Errorf("expected ownership recorded for %q", path)
			continue
		}
		if got.User != want.User || got.Group != want.Group {
			t.Errorf("ownership for %q: got %s:%s, want %s:%s",
				path, got.User, got.Group, want.User, want.Group)
		}
	}

	for path, wantPermStr := range expect.Perms {
		wantPerm := parsePermOrDie(t, wantPermStr)
		gotPerm, ok := tgt.Modes[path]
		if !ok {
			t.Errorf("expected target file %q to have permissions", path)
			continue
		}
		if gotPerm != wantPerm {
			t.Errorf("target file %q perms: got %o, want %o", path, gotPerm, wantPerm)
		}
	}

	for link, wantTarget := range expect.Symlinks {
		gotTarget, ok := tgt.Symlinks[link]
		if !ok {
			t.Errorf("expected symlink %q to exist", link)
			continue
		}
		if gotTarget != wantTarget {
			t.Errorf("symlink %q: got target %q, want %q", link, gotTarget, wantTarget)
		}
	}

	for pkg, wantInstalled := range expect.Pkgs {
		if gotInstalled := tgt.Pkgs[pkg]; gotInstalled != wantInstalled {
			t.Errorf("package %q: got installed=%v, want installed=%v", pkg, gotInstalled, wantInstalled)
		}
	}

	for svc, wantActive := range expect.Services {
		if gotActive := tgt.Services[svc]; gotActive != wantActive {
			t.Errorf("service %q: got active=%v, want active=%v", svc, gotActive, wantActive)
		}
	}
	for svc, wantEnabled := range expect.EnabledServices {
		if gotEnabled := tgt.EnabledServices[svc]; gotEnabled != wantEnabled {
			t.Errorf("service %q: got enabled=%v, want enabled=%v", svc, gotEnabled, wantEnabled)
		}
	}

	for svc, wantCount := range expect.Restarts {
		if gotCount := tgt.Restarts[svc]; gotCount != wantCount {
			t.Errorf("service %q: got %d restarts, want %d", svc, gotCount, wantCount)
		}
	}
	for svc, wantCount := range expect.Reloads {
		if gotCount := tgt.Reloads[svc]; gotCount != wantCount {
			t.Errorf("service %q: got %d reloads, want %d", svc, gotCount, wantCount)
		}
	}

	for name, wantInfo := range expect.Users {
		gotInfo, ok := tgt.Users[name]
		if !ok {
			t.Errorf("expected user %q to exist", name)
			continue
		}
		if wantInfo.Shell != "" && gotInfo.Shell != wantInfo.Shell {
			t.Errorf("user %q shell: got %q, want %q", name, gotInfo.Shell, wantInfo.Shell)
		}
		if wantInfo.Home != "" && gotInfo.Home != wantInfo.Home {
			t.Errorf("user %q home: got %q, want %q", name, gotInfo.Home, wantInfo.Home)
		}
		if wantInfo.System != gotInfo.System {
			t.Errorf("user %q system: got %v, want %v", name, gotInfo.System, wantInfo.System)
		}
		if len(wantInfo.Groups) > 0 {
			if !stringSlicesEqual(sorted(gotInfo.Groups), sorted(wantInfo.Groups)) {
				t.Errorf("user %q groups: got %v, want %v", name, gotInfo.Groups, wantInfo.Groups)
			}
		}
	}

	for name, wantConfigured := range expect.Repos {
		_, gotConfigured := tgt.Repos[name]
		if gotConfigured != wantConfigured {
			t.Errorf("repo %q: got configured=%v, want configured=%v", name, gotConfigured, wantConfigured)
		}
	}
	for name, wantInstalled := range expect.RepoKeys {
		if gotInstalled := tgt.RepoKeys[name]; gotInstalled != wantInstalled {
			t.Errorf("repo key %q: got installed=%v, want installed=%v", name, gotInstalled, wantInstalled)
		}
	}

	for name, wantInfo := range expect.Groups {
		gotInfo, ok := tgt.Groups[name]
		if !ok {
			t.Errorf("expected group %q to exist", name)
			continue
		}
		if wantInfo.GID != 0 && gotInfo.GID != wantInfo.GID {
			t.Errorf("group %q gid: got %d, want %d", name, gotInfo.GID, wantInfo.GID)
		}
		if wantInfo.System != gotInfo.System {
			t.Errorf("group %q system: got %v, want %v", name, gotInfo.System, wantInfo.System)
		}
	}
}

func sorted(s []string) []string {
	cp := make([]string, len(s))
	copy(cp, s)
	sort.Strings(cp)
	return cp
}
