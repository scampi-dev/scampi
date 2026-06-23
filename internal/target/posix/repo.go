// SPDX-License-Identifier: GPL-3.0-only

package posix

import (
	"strings"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/pkgmgr"
)

func (b Base) RepoKeyPath(name string) string {
	switch b.PkgBackend.Kind {
	case pkgmgr.Apt:
		return "/usr/share/keyrings/" + name + ".gpg"
	case pkgmgr.Dnf:
		return "/etc/pki/rpm-gpg/scampi-" + name + ".gpg"
	default:
		panic(errs.BUG("RepoKeyPath called on unsupported backend %s", b.PkgBackend.Kind))
	}
}

func (b Base) RepoConfigPath(name string) string {
	switch b.PkgBackend.Kind {
	case pkgmgr.Apt:
		return "/etc/apt/sources.list.d/scampi-" + name + ".sources"
	case pkgmgr.Dnf:
		return "/etc/yum.repos.d/scampi-" + name + ".repo"
	default:
		panic(errs.BUG("RepoConfigPath called on unsupported backend %s", b.PkgBackend.Kind))
	}
}

// BuildRepoContent generates the repo config file content for the given backend.
func BuildRepoContent(kind pkgmgr.Kind, cfg target.RepoConfig) string {
	switch kind {
	case pkgmgr.Apt:
		return buildDEB822(cfg)
	case pkgmgr.Dnf:
		return buildDNFRepo(cfg)
	default:
		panic(errs.BUG("BuildRepoContent called on unsupported backend %s", kind))
	}
}

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
