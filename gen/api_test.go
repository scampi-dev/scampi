// SPDX-License-Identifier: GPL-3.0-only

package gen

import (
	"testing"

	"scampi.dev/scampi/lang/token"
)

func TestKeywordReplacementsCoverAllKeywords(t *testing.T) {
	for kw := range token.Keywords {
		if _, ok := keywordReplacements[kw]; !ok {
			t.Errorf("keyword %q has no entry in keywordReplacements", kw)
		}
	}
}

func TestKeywordReplacementsAreNotKeywords(t *testing.T) {
	for kw, replacement := range keywordReplacements {
		if _, ok := token.Keywords[replacement]; ok {
			t.Errorf("replacement for %q is itself a keyword: %q", kw, replacement)
		}
	}
}
