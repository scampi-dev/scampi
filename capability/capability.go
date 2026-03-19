// SPDX-License-Identifier: GPL-3.0-only

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
	Filesystem Capability = 0b1 << iota // ReadFile, WriteFile, Stat, Remove
	FileMode                            // Chmod, mode in Stat
	Symlink                             // Symlink, Readlink, Lstat
	Ownership                           // HasUser, HasGroup, GetOwner, Chown
	Pkg                                 // IsInstalled, InstallPkgs, RemovePkgs
	PkgUpdate                           // UpdateCache, IsUpgradable
	Service                             // IsActive, IsEnabled, Start, Stop, Enable, Disable
	Command                             // RunCommand
	User                                // UserExists, CreateUser, ModifyUser, DeleteUser, GetUser
	Group                               // GroupExists, CreateGroup, DeleteGroup, GetGroup
	PkgRepo                             // HasRepo, HasRepoKey, InstallRepoKey, WriteRepoConfig
)

const (
	POSIX Capability = Filesystem | Ownership | FileMode | Symlink | Command | User | Group
)

func (c Capability) HasAll(other Capability) bool {
	return c&other == other
}

func (c Capability) HasAny(other Capability) bool {
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
	if c&PkgUpdate != 0 {
		parts = append(parts, "PkgUpdate")
	}
	if c&Service != 0 {
		parts = append(parts, "Service")
	}
	if c&Command != 0 {
		parts = append(parts, "Command")
	}
	if c&User != 0 {
		parts = append(parts, "User")
	}
	if c&Group != 0 {
		parts = append(parts, "Group")
	}
	if c&PkgRepo != 0 {
		parts = append(parts, "PkgRepo")
	}

	// If no known flags matched, show raw value
	if len(parts) == 0 {
		return fmt.Sprintf("Capability(%d)", c)
	}

	return strings.Join(parts, ", ")
}
