// SPDX-License-Identifier: GPL-3.0-only

package ansi

import (
	"slices"
	"strconv"
	"strings"
)

// ANSI represents a condensed SGR sequence builder
type ANSI struct {
	params []int
}

const (
	Reset = "\x1b[0m"
)

// Internals
// -----------------------------------------------------------------------------

func (a ANSI) add(p int) ANSI {
	if slices.Contains(a.params, p) {
		return a
	}
	c := make([]int, len(a.params)+1)
	copy(c, a.params)
	c[len(a.params)] = p
	return ANSI{params: c}
}

func (a ANSI) String() string {
	if len(a.params) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\x1b[")

	for i, p := range a.params {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(strconv.Itoa(p))
	}

	b.WriteByte('m')
	return b.String()
}

// Wrap applies the style and resets after
func (a ANSI) Wrap(s string) string {
	return a.String() + s + Reset
}

// Styles
// -----------------------------------------------------------------------------

func (a ANSI) Bold() ANSI      { return a.add(1) }
func (a ANSI) Dim() ANSI       { return a.add(2) }
func (a ANSI) Underline() ANSI { return a.add(4) }
func (a ANSI) Reverse() ANSI   { return a.add(7) }

// Foreground colors
// -----------------------------------------------------------------------------

func Black() ANSI   { return ANSI{}.add(30) }
func Red() ANSI     { return ANSI{}.add(31) }
func Green() ANSI   { return ANSI{}.add(32) }
func Yellow() ANSI  { return ANSI{}.add(33) }
func Blue() ANSI    { return ANSI{}.add(34) }
func Magenta() ANSI { return ANSI{}.add(35) }
func Cyan() ANSI    { return ANSI{}.add(36) }
func White() ANSI   { return ANSI{}.add(37) }

// Bright foreground colors
// -----------------------------------------------------------------------------

func BrightBlack() ANSI   { return ANSI{}.add(90) }
func BrightRed() ANSI     { return ANSI{}.add(91) }
func BrightGreen() ANSI   { return ANSI{}.add(92) }
func BrightYellow() ANSI  { return ANSI{}.add(93) }
func BrightBlue() ANSI    { return ANSI{}.add(94) }
func BrightMagenta() ANSI { return ANSI{}.add(95) }
func BrightCyan() ANSI    { return ANSI{}.add(96) }
func BrightWhite() ANSI   { return ANSI{}.add(97) }

// Background colors
// -----------------------------------------------------------------------------

func (a ANSI) BgBlack() ANSI   { return a.add(40) }
func (a ANSI) BgRed() ANSI     { return a.add(41) }
func (a ANSI) BgGreen() ANSI   { return a.add(42) }
func (a ANSI) BgYellow() ANSI  { return a.add(43) }
func (a ANSI) BgBlue() ANSI    { return a.add(44) }
func (a ANSI) BgMagenta() ANSI { return a.add(45) }
func (a ANSI) BgCyan() ANSI    { return a.add(46) }
func (a ANSI) BgWhite() ANSI   { return a.add(47) }

// Bright background colors
// -----------------------------------------------------------------------------

func (a ANSI) BgBrightBlack() ANSI   { return a.add(100) }
func (a ANSI) BgBrightRed() ANSI     { return a.add(101) }
func (a ANSI) BgBrightGreen() ANSI   { return a.add(102) }
func (a ANSI) BgBrightYellow() ANSI  { return a.add(103) }
func (a ANSI) BgBrightBlue() ANSI    { return a.add(104) }
func (a ANSI) BgBrightMagenta() ANSI { return a.add(105) }
func (a ANSI) BgBrightCyan() ANSI    { return a.add(106) }
func (a ANSI) BgBrightWhite() ANSI   { return a.add(107) }

// Cursor control
// -----------------------------------------------------------------------------

const (
	EraseLine  = "\x1b[2K"
	EraseToEnd = "\x1b[J"
)

func CursorUp(n int) string {
	if n <= 0 {
		return ""
	}
	return "\x1b[" + strconv.Itoa(n) + "A"
}
