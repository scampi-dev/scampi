// SPDX-License-Identifier: GPL-3.0-only

package engine

import "fmt"

// hintSuffix returns a " (did you mean ...)" fragment if a close
// candidate exists, or "" otherwise. Built so call sites can append
// to an existing message format.
func hintSuffix(s string, candidates []string) string {
	h := suggest(s, candidates)
	if h == "" {
		return ""
	}
	return fmt.Sprintf(" (did you mean %q?)", h)
}

func schemaAttrNames(sch KindSchema) []string {
	out := make([]string, len(sch))
	for i, spec := range sch {
		out[i] = spec.Name
	}
	return out
}

// suggest returns the closest candidate to s by Levenshtein
// distance, or "" if none is within threshold. The threshold scales
// with s's rune length so short identifiers don't suggest wildly
// different alternatives.
func suggest(s string, candidates []string) string {
	threshold := max(1, len([]rune(s))/3)
	best := ""
	bestDist := -1
	for _, c := range candidates {
		d := levenshtein(s, c)
		if d > threshold {
			continue
		}
		if bestDist == -1 || d < bestDist {
			best = c
			bestDist = d
		}
	}
	return best
}

// levenshtein returns the edit distance between a and b. Runs in
// O(len(a)*len(b)) time with O(len(b)) memory (two rolling rows).
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
