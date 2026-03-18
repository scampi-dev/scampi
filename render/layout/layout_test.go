// SPDX-License-Identifier: GPL-3.0-only

package layout

import (
	"testing"
)

func TestWrapText(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		maxLen int
		want   []string
	}{
		{
			name:   "fits in one line",
			text:   "hello world",
			maxLen: 20,
			want:   []string{"hello world"},
		},
		{
			name:   "exact fit",
			text:   "hello world",
			maxLen: 11,
			want:   []string{"hello world"},
		},
		{
			name:   "wraps at word boundary",
			text:   "hello world",
			maxLen: 8,
			want:   []string{"hello", "world"},
		},
		{
			name:   "multiple wraps",
			text:   "one two three four five",
			maxLen: 10,
			want:   []string{"one two", "three four", "five"},
		},
		{
			name:   "long word exceeds max",
			text:   "supercalifragilistic",
			maxLen: 10,
			want:   []string{"supercalifragilistic"},
		},
		{
			name:   "empty string",
			text:   "",
			maxLen: 10,
			want:   []string{""},
		},
		{
			name:   "zero maxLen returns as-is",
			text:   "hello world",
			maxLen: 0,
			want:   []string{"hello world"},
		},
		{
			name:   "single word",
			text:   "hello",
			maxLen: 3,
			want:   []string{"hello"},
		},
		{
			name:   "multiple spaces preserved when fits",
			text:   "hello   world",
			maxLen: 20,
			want:   []string{"hello   world"},
		},
		{
			name:   "multiple spaces collapsed on wrap",
			text:   "hello   world   foo",
			maxLen: 12,
			want:   []string{"hello world", "foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapText(tt.text, tt.maxLen)
			if len(got) != len(tt.want) {
				t.Fatalf("WrapText(%q, %d) = %v, want %v", tt.text, tt.maxLen, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
