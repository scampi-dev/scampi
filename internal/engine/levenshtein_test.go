// SPDX-License-Identifier: GPL-3.0-only

package engine

import "testing"

func TestSuggest_ClosestMatchInsideThreshold(t *testing.T) {
	cases := []struct {
		name string
		in   string
		cand []string
		want string
	}{
		{"single-char typo", "contnet", []string{"content", "path"}, "content"},
		{"trailing-s drift", "contents", []string{"content", "path"}, "content"},
		{"no close match", "blah_unrelated", []string{"content", "path"}, ""},
		{"empty candidates", "foo", nil, ""},
		{"exact match", "path", []string{"path", "content"}, "path"},
		{"closest of many", "pat", []string{"path", "patio", "park"}, "path"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := suggest(c.in, c.cand); got != c.want {
				t.Errorf("suggest(%q, %v) = %q, want %q", c.in, c.cand, got, c.want)
			}
		})
	}
}
