// SPDX-License-Identifier: GPL-3.0-only

package render

import "testing"

func TestDecideGlyphs(t *testing.T) {
	cases := []struct {
		name      string
		flag      bool
		env       string
		wantASCII bool
	}{
		{"default unicode", false, "", false},
		{"flag forces ascii", true, "", true},
		{"env forces ascii", false, "1", true},
		{"flag wins when env unset", true, "", true},
		{"env other than 1 ignored", false, "yes", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("SCAMPI_ASCII", c.env)
			got := DecideGlyphs(c.flag)
			gotASCII := got == ASCIIGlyphs
			if gotASCII != c.wantASCII {
				t.Errorf("got ascii=%v, want %v", gotASCII, c.wantASCII)
			}
		})
	}
}
