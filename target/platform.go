// SPDX-License-Identifier: GPL-3.0-only

package target

import (
	"strings"
)

// Platform represents the target operating system at the granularity needed
// for business logic: package manager selection and GNU vs BSD dispatch.
type Platform uint8

const (
	PlatformUnknown Platform = iota
	PlatformDarwin
	PlatformFreeBSD
	PlatformDebian
	PlatformUbuntu
	PlatformAlpine
	PlatformFedora
	PlatformRHEL
	PlatformArch
	PlatformSUSE
)

func (p Platform) String() string {
	switch p {
	case PlatformDarwin:
		return "darwin"
	case PlatformFreeBSD:
		return "freebsd"
	case PlatformDebian:
		return "debian"
	case PlatformUbuntu:
		return "ubuntu"
	case PlatformAlpine:
		return "alpine"
	case PlatformFedora:
		return "fedora"
	case PlatformRHEL:
		return "rhel"
	case PlatformArch:
		return "arch"
	case PlatformSUSE:
		return "suse"
	default:
		return "unknown"
	}
}

// IsGNU reports whether the platform uses GNU coreutils (stat, etc.).
func (p Platform) IsGNU() bool {
	switch p {
	case PlatformDebian, PlatformUbuntu, PlatformAlpine,
		PlatformFedora, PlatformRHEL, PlatformArch, PlatformSUSE:
		return true
	default:
		return false
	}
}

// IsBSD reports whether the platform uses BSD coreutils (stat, etc.).
func (p Platform) IsBSD() bool {
	switch p {
	case PlatformDarwin, PlatformFreeBSD:
		return true
	default:
		return false
	}
}

// OSInfo holds identification data for a target operating system.
type OSInfo struct {
	Platform        Platform
	VersionCodename string // e.g. "bookworm", "jammy" (from os-release)
}

// parsePlatformFromID maps an os-release ID or ID_LIKE value to a Platform.
func parsePlatformFromID(s string) Platform {
	switch s {
	case "debian":
		return PlatformDebian
	case "ubuntu":
		return PlatformUbuntu
	case "alpine":
		return PlatformAlpine
	case "fedora":
		return PlatformFedora
	case "rhel":
		return PlatformRHEL
	case "arch":
		return PlatformArch
	case "suse":
		return PlatformSUSE
	default:
		return PlatformUnknown
	}
}

// ParseKernel converts uname -s output to a Platform.
// For Linux, the caller should refine via ResolveLinuxPlatform.
func ParseKernel(s string) Platform {
	switch s {
	case "Linux":
		return PlatformUnknown // needs distro detection
	case "Darwin":
		return PlatformDarwin
	case "FreeBSD":
		return PlatformFreeBSD
	default:
		return PlatformUnknown
	}
}

// ResolveLinuxPlatform parses /etc/os-release content and returns the
// resolved Platform and version codename. Tries ID first, then ID_LIKE.
func ResolveLinuxPlatform(content []byte) OSInfo {
	var id string
	var idLike []string
	var codename string

	for line := range strings.SplitSeq(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		val = strings.Trim(val, `"`)

		switch key {
		case "ID":
			id = val
		case "ID_LIKE":
			idLike = strings.Fields(val)
		case "VERSION_CODENAME":
			codename = val
		}
	}

	// Resolve: try ID first, then ID_LIKE entries.
	if p := parsePlatformFromID(id); p != PlatformUnknown {
		return OSInfo{Platform: p, VersionCodename: codename}
	}
	for _, like := range idLike {
		if p := parsePlatformFromID(like); p != PlatformUnknown {
			return OSInfo{Platform: p, VersionCodename: codename}
		}
	}

	return OSInfo{VersionCodename: codename}
}
