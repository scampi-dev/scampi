// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"context"
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/pkgmgr"
)

// OSInfoProvider
// -----------------------------------------------------------------------------

func (t *SSHTarget) VersionCodename() string {
	return t.osInfo.VersionCodename
}

// RepoManager — apt backend
// -----------------------------------------------------------------------------

func (t *SSHTarget) HasRepoKey(_ context.Context, name string) (bool, error) {
	path := t.RepoKeyPath(name)
	_, err := t.sftp.Stat(path)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, normalizeError(err)
	}
	return true, nil
}

func (t *SSHTarget) InstallRepoKey(ctx context.Context, cfg target.RepoConfig) error {
	return t.WriteFile(ctx, cfg.KeyPath, cfg.KeyData)
}

func (t *SSHTarget) HasRepo(_ context.Context, name string) (bool, error) {
	path := t.RepoConfigPath(name)
	_, err := t.sftp.Stat(path)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, normalizeError(err)
	}
	return true, nil
}

func (t *SSHTarget) WriteRepoConfig(ctx context.Context, cfg target.RepoConfig) error {
	var content string
	switch t.pkgBackend.Kind {
	case pkgmgr.Apt:
		content = buildDEB822(cfg)
	case pkgmgr.Dnf:
		content = buildDNFRepo(cfg)
	default:
		panic(errs.BUG("WriteRepoConfig called on unsupported backend %s", t.pkgBackend.Kind))
	}
	return t.WriteFile(ctx, cfg.ConfigPath, []byte(content))
}

func (t *SSHTarget) RemoveRepo(ctx context.Context, name string) error {
	keyPath := t.RepoKeyPath(name)
	cfgPath := t.RepoConfigPath(name)
	_ = t.Remove(ctx, keyPath)
	return t.Remove(ctx, cfgPath)
}

func (t *SSHTarget) RepoKeyPath(name string) string {
	switch t.pkgBackend.Kind {
	case pkgmgr.Apt:
		return "/usr/share/keyrings/" + name + ".gpg"
	case pkgmgr.Dnf:
		return "/etc/pki/rpm-gpg/scampi-" + name + ".gpg"
	default:
		panic(errs.BUG("RepoKeyPath called on unsupported backend %s", t.pkgBackend.Kind))
	}
}

func (t *SSHTarget) RepoConfigPath(name string) string {
	switch t.pkgBackend.Kind {
	case pkgmgr.Apt:
		return "/etc/apt/sources.list.d/scampi-" + name + ".sources"
	case pkgmgr.Dnf:
		return "/etc/yum.repos.d/scampi-" + name + ".repo"
	default:
		panic(errs.BUG("RepoConfigPath called on unsupported backend %s", t.pkgBackend.Kind))
	}
}

// buildDEB822 generates a DEB822-format .sources file for apt.
func buildDEB822(cfg target.RepoConfig) string {
	var b strings.Builder
	b.WriteString("Types: deb\n")
	b.WriteString("URIs: " + cfg.URL + "\n")
	b.WriteString("Suites: " + cfg.Suite + "\n")
	b.WriteString("Components: " + strings.Join(cfg.Components, " ") + "\n")
	if cfg.KeyPath != "" {
		b.WriteString("Signed-By: " + cfg.KeyPath + "\n")
	}
	return b.String()
}

// buildDNFRepo generates an INI-format .repo file for dnf/yum.
func buildDNFRepo(cfg target.RepoConfig) string {
	var b strings.Builder
	b.WriteString("[scampi-" + cfg.Name + "]\n")
	b.WriteString("name=scampi-" + cfg.Name + "\n")
	b.WriteString("baseurl=" + cfg.URL + "\n")
	b.WriteString("enabled=1\n")
	if cfg.KeyPath != "" {
		b.WriteString("gpgcheck=1\n")
		b.WriteString("gpgkey=file://" + cfg.KeyPath + "\n")
	} else {
		b.WriteString("gpgcheck=0\n")
	}
	return b.String()
}
