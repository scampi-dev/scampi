package capability

import (
	"fmt"
	"strings"
)

type Capability uint16

const (
	None Capability = 0
	All  Capability = ^Capability(0)
)
const (
	Filesystem Capability = 1 << iota // ReadFile, WriteFile, Stat, Remove
	FileMode                          // Chmod, mode in Stat
	Symlink                           // Symlink, Readlink, Lstat
	Ownership                         // HasUser, HasGroup, GetOwner, Chown
	Pkg                               // IsInstalled, InstallPkgs, RemovePkgs
)

const (
	// Full POSIX filesystem
	POSIX Capability = Filesystem | Ownership | FileMode | Symlink
)

func (c Capability) Has(other Capability) bool {
	return c&other != 0
}

func (c Capability) String() string {
	// Special cases
	switch c {
	case 0:
		return "None"
	case POSIX:
		return "POSIX"
	}

	var parts []string

	if c&Filesystem != 0 {
		parts = append(parts, "Filesystem")
	}
	if c&Ownership != 0 {
		parts = append(parts, "Ownership")
	}
	if c&FileMode != 0 {
		parts = append(parts, "FileMode")
	}
	if c&Symlink != 0 {
		parts = append(parts, "Symlink")
	}
	if c&Pkg != 0 {
		parts = append(parts, "Pkg")
	}

	// If no known flags matched, show raw value
	if len(parts) == 0 {
		return fmt.Sprintf("Capability(%d)", c)
	}

	return strings.Join(parts, ", ")
}
