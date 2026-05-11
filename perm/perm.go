// SPDX-License-Identifier: GPL-3.0-only

// Package perm parses file permission literals in octal, ls-style, or
// POSIX format and returns the corresponding fs.FileMode.
package perm

import (
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

const CodeInvalidPermission errs.Code = "step.InvalidPermission"

type InvalidPermissionError struct {
	diagnostic.FatalError
	Value  string
	Hint   string
	Source spec.SourceSpan
}

func (e InvalidPermissionError) Error() string {
	return fmt.Sprintf("invalid permission %q", e.Value)
}

func (e InvalidPermissionError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeInvalidPermission,
		Text: "invalid file permission '{{.Value}}'",
		Hint: "expected octal, ls-style, or posix permissions",
		Help: `supported formats:
  - octal:        0600, 0644, 0755
  - ls-style:     rw-r--r--
  - posix style:  u=rw,g=r,o=r`,
		Data:   e,
		Source: &e.Source,
	}
}

var (
	octalRe = regexp.MustCompile(`^0[0-7]{3}$`)
	lsRe    = regexp.MustCompile(`^[r-][w-][x-][r-][w-][x-][r-][w-][x-]$`)
	posixRe = regexp.MustCompile(`^(u|g|o)=[rwx]*(,(u|g|o)=[rwx]*)*$`)
)

func ParsePerm(s string, src spec.SourceSpan) (fs.FileMode, error) {
	if m, ok := tryOctal(s); ok {
		return m, nil
	}
	if m, ok := tryLs(s); ok {
		return m, nil
	}
	if m, ok := tryPosix(s); ok {
		return m, nil
	}

	return 0, InvalidPermissionError{
		Value:  s,
		Source: src,
	}
}

func tryOctal(s string) (fs.FileMode, bool) {
	if !octalRe.MatchString(s) {
		return 0, false
	}

	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, false
	}

	return fs.FileMode(v) & fs.ModePerm, true
}

func tryLs(s string) (fs.FileMode, bool) {
	if !lsRe.MatchString(s) {
		return 0, false
	}

	var mode fs.FileMode

	triples := []struct {
		offset int
		shift  uint
	}{
		{0, 6}, // user
		{3, 3}, // group
		{6, 0}, // other
	}

	for _, t := range triples {
		var bits fs.FileMode

		if s[t.offset] == 'r' {
			bits |= 4
		}
		if s[t.offset+1] == 'w' {
			bits |= 2
		}
		if s[t.offset+2] == 'x' {
			bits |= 1
		}

		mode |= bits << t.shift
	}

	return mode & fs.ModePerm, true
}

func tryPosix(s string) (fs.FileMode, bool) {
	if !posixRe.MatchString(s) {
		return 0, false
	}

	seen := map[byte]bool{}
	var mode fs.FileMode

	for c := range strings.SplitSeq(s, ",") {
		who := c[0]
		if seen[who] {
			return 0, false
		}
		seen[who] = true

		var bits fs.FileMode
		for _, r := range c[2:] {
			switch r {
			case 'r':
				bits |= 4
			case 'w':
				bits |= 2
			case 'x':
				bits |= 1
			default:
				return 0, false
			}
		}

		shift := map[byte]uint{'u': 6, 'g': 3, 'o': 0}[who]
		mode |= bits << shift
	}

	if len(seen) != 3 {
		return 0, false
	}

	return mode & fs.ModePerm, true
}
