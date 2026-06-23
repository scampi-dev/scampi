// SPDX-License-Identifier: GPL-3.0-only

package token

import (
	"unicode/utf8"
)

// Span is a byte-offset range into the source. Start is inclusive,
// End is exclusive. Both are byte offsets, not rune offsets — indexing
// into the source slice is O(1). Resolve to line/column via Resolve().
type Span struct {
	Start uint32
	End   uint32
}

// Pos is a resolved source position. Line and Col are 1-based and
// measured in runes from the start of the line; Offset is the byte
// position in the source. A zero Pos means "unknown".
type Pos struct {
	Line   int
	Col    int
	Offset uint32
}

// Resolve converts a byte offset into a line/column position by
// scanning the source from the start. Line numbers are 1-based;
// columns count runes (not bytes) from the start of the line, so
// multi-byte characters count as one column. If offset is past the
// end of source, the position of the last byte is returned.
//
// Cost: O(offset). Only call this on slow paths (diagnostics).
func Resolve(source []byte, offset uint32) Pos {
	if int(offset) > len(source) {
		offset = uint32(len(source))
	}
	line := 1
	lineStart := 0
	for i := 0; i < int(offset); i++ {
		if source[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	// Column: count runes from lineStart to offset.
	col := 1
	for i := lineStart; i < int(offset); {
		_, size := utf8.DecodeRune(source[i:])
		i += size
		col++
	}
	return Pos{Line: line, Col: col, Offset: offset}
}

// ResolveSpan resolves both ends of a span in a single pass when the
// span is entirely in the same contiguous region. For most spans this
// is cheaper than two independent Resolve calls.
func ResolveSpan(source []byte, s Span) (start, end Pos) {
	start = Resolve(source, s.Start)
	// Scan from s.Start to s.End, continuing the line/col tracking.
	line, col := start.Line, start.Col
	for i := int(s.Start); i < int(s.End) && i < len(source); {
		if source[i] == '\n' {
			line++
			col = 1
			i++
			continue
		}
		_, size := utf8.DecodeRune(source[i:])
		i += size
		col++
	}
	end = Pos{Line: line, Col: col, Offset: s.End}
	return start, end
}
