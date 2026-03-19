// SPDX-License-Identifier: GPL-3.0-only

// Package pkgmgr provides package manager detection and command templates.
package pkgmgr

//go:generate stringer -type=Kind -linecomment

// Kind identifies a package manager backend.
type Kind uint8

const (
	Brew   Kind = iota + 1 // brew
	Pkg                    // pkg
	Apt                    // apt
	Dnf                    // dnf
	Apk                    // apk
	Pacman                 // pacman
	Zypper                 // zypper
)

// Backend holds command templates for a package manager.
// Each template contains a single %s verb for the package name(s).
type Backend struct {
	Kind           Kind
	IsInstalled    string // exit 0 = installed
	Install        string
	Remove         string
	NeedsRoot      bool   // true when install/remove require root privileges
	IsUpgradable   string // exit 0 = upgradable (single %s for pkg name)
	UpdateCache    string // command to refresh package index
	CacheNeedsRoot bool   // whether cache refresh needs root
	CheckCacheAge  string // command that prints cache mtime as unix epoch; empty = unknown
}

func (b *Backend) SupportsUpgrade() bool {
	return b.UpdateCache != "" && b.IsUpgradable != ""
}

func (b *Backend) SupportsRepoSetup() bool {
	return b.Kind == Apt || b.Kind == Dnf
}

// backendsByFamily maps os-release ID/ID_LIKE values (and lowercased kernel
// names like "darwin") to backends.
var backendsByFamily = map[string]Backend{
	"darwin": {
		Kind:           Brew,
		IsInstalled:    "brew list %s",
		Install:        "brew install %s",
		Remove:         "brew uninstall %s",
		NeedsRoot:      false,
		IsUpgradable:   "brew outdated %s | grep -q .",
		UpdateCache:    "brew update --quiet",
		CacheNeedsRoot: false,
		CheckCacheAge:  "stat -f %m \"$(brew --repository)/.git/HEAD\" 2>/dev/null",
	},
	"freebsd": {
		Kind:           Pkg,
		IsInstalled:    "pkg info %s",
		Install:        "pkg install -y %s",
		Remove:         "pkg remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "pkg upgrade -n %s 2>/dev/null | grep -q .",
		UpdateCache:    "pkg update -q",
		CacheNeedsRoot: true,
		CheckCacheAge:  "stat -f %m /var/db/pkg/repo-FreeBSD.sqlite 2>/dev/null",
	},
	"debian": {
		Kind:           Apt,
		IsInstalled:    "dpkg -s %s 2>/dev/null | grep -q '^Status:.* installed$'",
		Install:        "DEBIAN_FRONTEND=noninteractive apt-get install -y %s",
		Remove:         "DEBIAN_FRONTEND=noninteractive apt-get remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "apt list --upgradable %s 2>/dev/null | grep -q .",
		UpdateCache:    "apt-get update -qq",
		CacheNeedsRoot: true,
		CheckCacheAge:  "stat -c %Y /var/cache/apt/pkgcache.bin 2>/dev/null",
	},
	"ubuntu": {
		Kind:           Apt,
		IsInstalled:    "dpkg -s %s 2>/dev/null | grep -q '^Status:.* installed$'",
		Install:        "DEBIAN_FRONTEND=noninteractive apt-get install -y %s",
		Remove:         "DEBIAN_FRONTEND=noninteractive apt-get remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "apt list --upgradable %s 2>/dev/null | grep -q .",
		UpdateCache:    "apt-get update -qq",
		CacheNeedsRoot: true,
		CheckCacheAge:  "stat -c %Y /var/cache/apt/pkgcache.bin 2>/dev/null",
	},
	"alpine": {
		Kind:           Apk,
		IsInstalled:    "apk info -e %s",
		Install:        "apk add %s",
		Remove:         "apk del %s",
		NeedsRoot:      true,
		IsUpgradable:   "apk version -l '<' %s 2>/dev/null | tail -n +2 | grep -q .",
		UpdateCache:    "apk update -q",
		CacheNeedsRoot: true,
		CheckCacheAge:  "stat -c %Y /var/cache/apk/APKINDEX.*.tar.gz 2>/dev/null | sort -n | tail -1",
	},
	"fedora": {
		Kind:           Dnf,
		IsInstalled:    "rpm -q %s",
		Install:        "dnf install -y %s",
		Remove:         "dnf remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "dnf check-update %s >/dev/null 2>&1; [ $? -eq 100 ]",
		UpdateCache:    "dnf makecache -q",
		CacheNeedsRoot: true,
		CheckCacheAge:  "stat -c %Y /var/cache/dnf/*/repodata/repomd.xml 2>/dev/null | sort -n | tail -1",
	},
	"rhel": {
		Kind:           Dnf,
		IsInstalled:    "rpm -q %s",
		Install:        "dnf install -y %s",
		Remove:         "dnf remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "dnf check-update %s >/dev/null 2>&1; [ $? -eq 100 ]",
		UpdateCache:    "dnf makecache -q",
		CacheNeedsRoot: true,
		CheckCacheAge:  "stat -c %Y /var/cache/dnf/*/repodata/repomd.xml 2>/dev/null | sort -n | tail -1",
	},
	"arch": {
		Kind:           Pacman,
		IsInstalled:    "pacman -Q %s",
		Install:        "pacman -S --noconfirm %s",
		Remove:         "pacman -R --noconfirm %s",
		NeedsRoot:      true,
		IsUpgradable:   "pacman -Qu %s >/dev/null 2>&1",
		UpdateCache:    "pacman -Sy --noconfirm",
		CacheNeedsRoot: true,
		CheckCacheAge:  "stat -c %Y /var/lib/pacman/sync/core.db 2>/dev/null",
	},
	"suse": {
		Kind:           Zypper,
		IsInstalled:    "rpm -q %s",
		Install:        "zypper install -y %s",
		Remove:         "zypper remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "zypper --non-interactive list-updates 2>/dev/null | grep -q %s",
		UpdateCache:    "zypper refresh -q",
		CacheNeedsRoot: true,
		CheckCacheAge:  "stat -c %Y /var/cache/zypp/solv/*/cookie 2>/dev/null | sort -n | tail -1",
	},
}
