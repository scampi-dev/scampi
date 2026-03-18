// SPDX-License-Identifier: GPL-3.0-only

// Package layout provides terminal layout utilities for renderers.
package layout

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// VisibleLen returns the visible width of a string, excluding ANSI escape codes.
func VisibleLen(s string) int {
	return runewidth.StringWidth(ansiRe.ReplaceAllString(s, ""))
}

// FitLine truncates a string to maxLen visible characters, preserving ANSI codes.
func FitLine(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}

	if VisibleLen(s) <= maxLen {
		return s
	}

	if maxLen == 1 {
		return "…"
	}

	var out strings.Builder
	var lastColor string
	width := 0

	for len(s) > 0 {
		if seq, ok := getANSI(s); ok {
			out.WriteString(seq)
			if seq != "\x1b[0m" {
				lastColor = seq
			}
			s = s[len(seq):]
			continue
		}

		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			break
		}

		rw := runewidth.RuneWidth(r)
		if width+rw >= maxLen {
			break
		}

		out.WriteRune(r)
		width += rw
		s = s[size:]
	}

	if lastColor != "" {
		out.WriteString(lastColor)
	}
	out.WriteString("…")

	return out.String()
}

func getANSI(s string) (seq string, ok bool) {
	if len(s) < 2 || s[0] != '\x1b' || s[1] != '[' {
		return "", false
	}
	for i := 2; i < len(s); i++ {
		if s[i] >= '@' && s[i] <= '~' {
			return s[:i+1], true
		}
	}
	return "", false
}

// WrapText splits plain text into lines of at most maxLen visible characters,
// breaking at word boundaries.
func WrapText(text string, maxLen int) []string {
	if maxLen <= 0 || runewidth.StringWidth(text) <= maxLen {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	current := words[0]
	currentWidth := runewidth.StringWidth(current)

	for _, word := range words[1:] {
		wordWidth := runewidth.StringWidth(word)
		if currentWidth+1+wordWidth > maxLen {
			lines = append(lines, current)
			current = word
			currentWidth = wordWidth
		} else {
			current += " " + word
			currentWidth += 1 + wordWidth
		}
	}
	lines = append(lines, current)

	return lines
}

// Plural returns "s" if n != 1, empty string otherwise.
func Plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
