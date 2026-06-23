// SPDX-License-Identifier: GPL-3.0-only

package cli

import "testing"

func TestCaretPadding(t *testing.T) {
	tests := []struct {
		name string
		line string
		col  int
		want string
	}{
		{
			name: "col zero",
			line: "hello",
			col:  0,
			want: "",
		},
		{
			name: "col one",
			line: "hello",
			col:  1,
			want: "",
		},
		{
			name: "spaces only",
			line: "hello world",
			col:  7,
			want: "      ",
		},
		{
			name: "tab preserved",
			line: "\thello",
			col:  3,
			want: "\t ",
		},
		{
			name: "multiple tabs",
			line: "\t\thello",
			col:  4,
			want: "\t\t ",
		},
		{
			name: "col beyond text length",
			line: "hi",
			col:  10,
			want: "  ",
		},
		{
			name: "mixed tabs and spaces",
			line: "  \thello",
			col:  5,
			want: "  \t ",
		},
		{
			name: "empty line col 1",
			line: "",
			col:  1,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := caretPadding(tt.line, tt.col)
			if got != tt.want {
				t.Errorf("caretPadding(%q, %d) = %q, want %q", tt.line, tt.col, got, tt.want)
			}
		})
	}
}

func TestUnderlineRange(t *testing.T) {
	tests := []struct {
		name  string
		start int
		end   int
		want  string
	}{
		{
			name:  "end less than start",
			start: 5,
			end:   3,
			want:  "^",
		},
		{
			name:  "end equals start",
			start: 5,
			end:   5,
			want:  "^",
		},
		{
			name:  "single char span",
			start: 5,
			end:   6,
			want:  "~",
		},
		{
			name:  "multi char span",
			start: 1,
			end:   5,
			want:  "~~~~",
		},
		{
			name:  "wide span",
			start: 0,
			end:   10,
			want:  "~~~~~~~~~~",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := underlineRange(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("underlineRange(%d, %d) = %q, want %q", tt.start, tt.end, got, tt.want)
			}
		})
	}
}
