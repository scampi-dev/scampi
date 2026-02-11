// Package pkgmgr provides package manager detection and command templates.
package pkgmgr

// Backend holds command templates for a package manager.
// Each template contains a single %s verb for the package name(s).
type Backend struct {
	Name        string
	IsInstalled string // exit 0 = installed
	Install     string
	Remove      string
}

// backendsByFamily maps os-release ID/ID_LIKE values (and lowercased kernel
// names like "darwin") to backends.
var backendsByFamily = map[string]Backend{
	"darwin": {
		Name:        "brew",
		IsInstalled: "brew list %s",
		Install:     "brew install %s",
		Remove:      "brew uninstall %s",
	},
	"freebsd": {
		Name:        "pkg",
		IsInstalled: "pkg info %s",
		Install:     "pkg install -y %s",
		Remove:      "pkg remove -y %s",
	},
	"debian": {
		Name:        "apt",
		IsInstalled: "dpkg -s %s",
		Install:     "DEBIAN_FRONTEND=noninteractive apt-get install -y %s",
		Remove:      "DEBIAN_FRONTEND=noninteractive apt-get remove -y %s",
	},
	"ubuntu": {
		Name:        "apt",
		IsInstalled: "dpkg -s %s",
		Install:     "DEBIAN_FRONTEND=noninteractive apt-get install -y %s",
		Remove:      "DEBIAN_FRONTEND=noninteractive apt-get remove -y %s",
	},
	"alpine": {
		Name:        "apk",
		IsInstalled: "apk info -e %s",
		Install:     "apk add %s",
		Remove:      "apk del %s",
	},
	"fedora": {
		Name:        "dnf",
		IsInstalled: "rpm -q %s",
		Install:     "dnf install -y %s",
		Remove:      "dnf remove -y %s",
	},
	"rhel": {
		Name:        "dnf",
		IsInstalled: "rpm -q %s",
		Install:     "dnf install -y %s",
		Remove:      "dnf remove -y %s",
	},
	"arch": {
		Name:        "pacman",
		IsInstalled: "pacman -Q %s",
		Install:     "pacman -S --noconfirm %s",
		Remove:      "pacman -R --noconfirm %s",
	},
	"suse": {
		Name:        "zypper",
		IsInstalled: "rpm -q %s",
		Install:     "zypper install -y %s",
		Remove:      "zypper remove -y %s",
	},
}
