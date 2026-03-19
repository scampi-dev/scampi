// SPDX-License-Identifier: GPL-3.0-only

package spec

// PkgSourceKind identifies the package source backend.
type PkgSourceKind uint8

const (
	PkgSourceNative PkgSourceKind = iota // system package manager, no repo setup
	PkgSourceApt
	PkgSourceDnf
)

// PkgSourceRef describes a third-party package repository to configure
// before installing packages.
type PkgSourceRef struct {
	Kind       PkgSourceKind
	Name       string // derived slug from URL (used as filename prefix)
	URL        string
	KeyURL     string
	Components []string // apt only
	Suite      string   // apt only (auto-detected if empty)
}
