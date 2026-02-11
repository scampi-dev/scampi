package pkgmgr

import (
	"strings"
)

// Kernel name constants returned by uname -s.
const (
	KernelLinux   = "Linux"
	KernelDarwin  = "Darwin"
	KernelFreeBSD = "FreeBSD"
)

// OSInfo holds identification data for a target operating system.
type OSInfo struct {
	Kernel string   // "Linux", "Darwin", etc. (from uname -s)
	ID     string   // e.g. "ubuntu", "alpine", "fedora" (from os-release)
	IDLike []string // e.g. ["debian"] for ubuntu (from os-release)
}

// ParseOSRelease parses /etc/os-release content into an OSInfo.
// The Kernel field is not set — the caller must populate it separately.
func ParseOSRelease(content []byte) OSInfo {
	var info OSInfo
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
			info.ID = val
		case "ID_LIKE":
			info.IDLike = strings.Fields(val)
		}
	}
	return info
}

// Detect returns the package manager backend for the given OS, or nil
// if no supported manager is known.
func Detect(info OSInfo) *Backend {
	// Non-Linux kernels are direct mappings — no distro detection needed.
	switch info.Kernel {
	case KernelDarwin, KernelFreeBSD:
		if b, ok := backendsByFamily[strings.ToLower(info.Kernel)]; ok {
			return &b
		}
		return nil
	}

	// Linux: check ID first (exact match), then ID_LIKE (family match).
	for _, id := range append([]string{info.ID}, info.IDLike...) {
		if b, ok := backendsByFamily[id]; ok {
			return &b
		}
	}
	return nil
}
