// SPDX-License-Identifier: GPL-3.0-only

package unarchive

import (
	"strings"

	"scampi.dev/scampi/internal/errs"
)

type archiveFormat int

const (
	formatTarGz archiveFormat = iota
	formatTarBz2
	formatTarXz
	formatTarZst
	formatTar
	formatZip
	_formatCount // sentinel — must be last
)

type formatEntry struct {
	ext    string
	format archiveFormat
}

// Longest-first so ".tar.gz" matches before ".gz".
var formatTable = []formatEntry{
	{".tar.gz", formatTarGz},
	{".tar.bz2", formatTarBz2},
	{".tar.xz", formatTarXz},
	{".tar.zst", formatTarZst},
	{".tgz", formatTarGz},
	{".tbz2", formatTarBz2},
	{".txz", formatTarXz},
	{".tzst", formatTarZst},
	{".tar", formatTar},
	{".zip", formatZip},
}

func detectFormat(path string) (archiveFormat, bool) {
	lower := strings.ToLower(path)
	for _, e := range formatTable {
		if strings.HasSuffix(lower, e.ext) {
			return e.format, true
		}
	}
	return 0, false
}

func (f archiveFormat) extractCmd(archivePath, dest string) string {
	switch f {
	case formatTarGz:
		return "tar xzf " + archivePath + " -C " + dest
	case formatTarBz2:
		return "tar xjf " + archivePath + " -C " + dest
	case formatTarXz:
		return "tar xJf " + archivePath + " -C " + dest
	case formatTarZst:
		return "tar --zstd -xf " + archivePath + " -C " + dest
	case formatTar:
		return "tar xf " + archivePath + " -C " + dest
	case formatZip:
		return "unzip -o " + archivePath + " -d " + dest
	default:
		panic(errs.BUG("unhandled archive format %d", f))
	}
}

func (f archiveFormat) requiredTool() string {
	if f == formatZip {
		return "unzip"
	}
	return "tar"
}

// archiveExtensions returns the glob pattern fragment for find(1).
func archiveExtensions() string {
	return `-name '*.tar.gz' -o -name '*.tgz' ` +
		`-o -name '*.tar.bz2' -o -name '*.tbz2' ` +
		`-o -name '*.tar.xz' -o -name '*.txz' ` +
		`-o -name '*.tar.zst' -o -name '*.tzst' ` +
		`-o -name '*.tar' -o -name '*.zip'`
}
