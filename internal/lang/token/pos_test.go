// SPDX-License-Identifier: GPL-3.0-only

package token

import "testing"

func TestResolve(t *testing.T) {
	src := []byte("hello\nworld\n  foo\n")
	//             ^0    ^6   ^12   ^18 (length)
	//             h=0   w=6   f=14
	//             e=1   o=7
	//             l=2   r=8
	//             l=3   l=9
	//             o=4   d=10
	//             \n=5  \n=11
	//                   (space)=12
	//                   (space)=13
	//                   f=14
	//                   o=15
	//                   o=16
	//                   \n=17

	cases := []struct {
		name     string
		offset   uint32
		wantLine int
		wantCol  int
	}{
		{"start", 0, 1, 1},
		{"mid line 1", 2, 1, 3},
		{"end of line 1", 5, 1, 6}, // the '\n' itself on line 1
		{"start of line 2", 6, 2, 1},
		{"end of line 2", 11, 2, 6},
		{"start of line 3", 12, 3, 1},
		{"after indent line 3", 14, 3, 3},
		{"past end", 100, 4, 1}, // offset clamped to len
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(src, tc.offset)
			if got.Line != tc.wantLine || got.Col != tc.wantCol {
				t.Errorf("offset=%d: got line=%d col=%d, want line=%d col=%d",
					tc.offset, got.Line, got.Col, tc.wantLine, tc.wantCol)
			}
		})
	}
}

func TestResolveMultiByte(t *testing.T) {
	// "α" (U+03B1) is 2 bytes in UTF-8; should count as 1 column.
	src := []byte("αβγ\nδ")
	// bytes: α=0,1  β=2,3  γ=4,5  \n=6  δ=7,8
	cases := []struct {
		offset   uint32
		wantLine int
		wantCol  int
	}{
		{0, 1, 1}, // before α
		{2, 1, 2}, // before β (after α)
		{4, 1, 3}, // before γ (after αβ)
		{6, 1, 4}, // before \n (after αβγ)
		{7, 2, 1}, // start of line 2 (δ)
	}
	for _, tc := range cases {
		got := Resolve(src, tc.offset)
		if got.Line != tc.wantLine || got.Col != tc.wantCol {
			t.Errorf("offset=%d: got line=%d col=%d, want line=%d col=%d",
				tc.offset, got.Line, got.Col, tc.wantLine, tc.wantCol)
		}
	}
}

func TestResolveSpan(t *testing.T) {
	src := []byte("foo\nbar baz\n")
	//             0123 456789...
	// span covers "bar baz" = bytes 4..11
	start, end := ResolveSpan(src, Span{Start: 4, End: 11})
	if start.Line != 2 || start.Col != 1 {
		t.Errorf("start: got line=%d col=%d, want 2,1", start.Line, start.Col)
	}
	if end.Line != 2 || end.Col != 8 {
		t.Errorf("end: got line=%d col=%d, want 2,8", end.Line, end.Col)
	}
}

func TestKeywordLookup(t *testing.T) {
	cases := []struct {
		in   string
		want Kind
	}{
		{"import", Import},
		{"let", Let},
		{"func", Func},
		{"decl", Decl},
		{"type", Type},
		{"enum", Enum},
		{"for", For},
		{"if", If},
		{"else", Else},
		{"return", Return},
		{"true", True},
		{"false", False},
		{"none", None},
		{"self", Self},
		{"foo", Ident},
		{"Decl", Ident}, // case-sensitive
		{"", Ident},
	}
	for _, tc := range cases {
		if got := Lookup(tc.in); got != tc.want {
			t.Errorf("Lookup(%q): got %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestEndsStatement(t *testing.T) {
	cases := []struct {
		k    Kind
		want bool
	}{
		{Ident, true},
		{Int, true},
		{String, true},
		{StringEnd, true},
		{True, true},
		{Return, true},
		{RBrace, true},
		{RBrack, true},
		{RParen, true},
		{Plus, false},
		{Comma, false},
		{LBrace, false},
		{Import, false},
	}
	for _, tc := range cases {
		if got := tc.k.EndsStatement(); got != tc.want {
			t.Errorf("EndsStatement(%v): got %v, want %v", tc.k, got, tc.want)
		}
	}
}
