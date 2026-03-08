// SPDX-License-Identifier: GPL-3.0-only

// Package pkgmgr provides package manager detection and command templates.
package pkgmgr

// Backend holds command templates for a package manager.
// Each template contains a single %s verb for the package name(s).
type Backend struct {
	Name           string
	IsInstalled    string // exit 0 = installed
	Install        string
	Remove         string
	NeedsRoot      bool   // true when install/remove require root privileges
	IsUpgradable   string // exit 0 = upgradable (single %s for pkg name)
	UpdateCache    string // command to refresh package index
	CacheNeedsRoot bool   // whether cache refresh needs root
}

func (b *Backend) SupportsUpgrade() bool {
	return b.UpdateCache != "" && b.IsUpgradable != ""
}

// backendsByFamily maps os-release ID/ID_LIKE values (and lowercased kernel
// names like "darwin") to backends.
var backendsByFamily = map[string]Backend{
	"darwin": {
		Name:           "brew",
		IsInstalled:    "brew list %s",
		Install:        "brew install %s",
		Remove:         "brew uninstall %s",
		NeedsRoot:      false,
		IsUpgradable:   "brew outdated %s | grep -q .",
		UpdateCache:    "brew update --quiet",
		CacheNeedsRoot: false,
	},
	"freebsd": {
		Name:           "pkg",
		IsInstalled:    "pkg info %s",
		Install:        "pkg install -y %s",
		Remove:         "pkg remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "pkg upgrade -n %s 2>/dev/null | grep -q .",
		UpdateCache:    "pkg update -q",
		CacheNeedsRoot: true,
	},
	"debian": {
		Name:           "apt",
		IsInstalled:    "dpkg -s %s",
		Install:        "DEBIAN_FRONTEND=noninteractive apt-get install -y %s",
		Remove:         "DEBIAN_FRONTEND=noninteractive apt-get remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "apt list --upgradable %s 2>/dev/null | grep -q .",
		UpdateCache:    "apt-get update -qq",
		CacheNeedsRoot: true,
	},
	"ubuntu": {
		Name:           "apt",
		IsInstalled:    "dpkg -s %s",
		Install:        "DEBIAN_FRONTEND=noninteractive apt-get install -y %s",
		Remove:         "DEBIAN_FRONTEND=noninteractive apt-get remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "apt list --upgradable %s 2>/dev/null | grep -q .",
		UpdateCache:    "apt-get update -qq",
		CacheNeedsRoot: true,
	},
	"alpine": {
		Name:           "apk",
		IsInstalled:    "apk info -e %s",
		Install:        "apk add %s",
		Remove:         "apk del %s",
		NeedsRoot:      true,
		IsUpgradable:   "apk version -l '<' %s 2>/dev/null | tail -n +2 | grep -q .",
		UpdateCache:    "apk update -q",
		CacheNeedsRoot: true,
	},
	"fedora": {
		Name:           "dnf",
		IsInstalled:    "rpm -q %s",
		Install:        "dnf install -y %s",
		Remove:         "dnf remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "dnf check-update %s >/dev/null 2>&1; [ $? -eq 100 ]",
		UpdateCache:    "dnf makecache -q",
		CacheNeedsRoot: true,
	},
	"rhel": {
		Name:           "dnf",
		IsInstalled:    "rpm -q %s",
		Install:        "dnf install -y %s",
		Remove:         "dnf remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "dnf check-update %s >/dev/null 2>&1; [ $? -eq 100 ]",
		UpdateCache:    "dnf makecache -q",
		CacheNeedsRoot: true,
	},
	"arch": {
		Name:           "pacman",
		IsInstalled:    "pacman -Q %s",
		Install:        "pacman -S --noconfirm %s",
		Remove:         "pacman -R --noconfirm %s",
		NeedsRoot:      true,
		IsUpgradable:   "pacman -Qu %s >/dev/null 2>&1",
		UpdateCache:    "pacman -Sy --noconfirm",
		CacheNeedsRoot: true,
	},
	"suse": {
		Name:           "zypper",
		IsInstalled:    "rpm -q %s",
		Install:        "zypper install -y %s",
		Remove:         "zypper remove -y %s",
		NeedsRoot:      true,
		IsUpgradable:   "zypper --non-interactive list-updates 2>/dev/null | grep -q %s",
		UpdateCache:    "zypper refresh -q",
		CacheNeedsRoot: true,
	},
}
