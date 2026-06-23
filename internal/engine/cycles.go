// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"strings"
)

// detectCycles finds all cycles reachable from the given roots using DFS
// back-edge detection. Each returned cycle is a slice where the last element
// repeats the first to close the loop.
func detectCycles[N comparable](roots []N, neighbors func(N) []N) [][]N {
	visited := map[N]bool{}
	onStack := map[N]bool{}

	var stack []N
	var cycles [][]N

	var dfs func(n N)
	dfs = func(n N) {
		if onStack[n] {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i] == n {
					cycle := append([]N{}, stack[i:]...)
					cycle = append(cycle, n)
					cycles = append(cycles, cycle)
					return
				}
			}
		}
		if visited[n] {
			return
		}

		visited[n] = true
		onStack[n] = true
		stack = append(stack, n)

		for _, next := range neighbors(n) {
			dfs(next)
		}

		stack = stack[:len(stack)-1]
		onStack[n] = false
	}

	for _, root := range roots {
		if !visited[root] {
			dfs(root)
		}
	}

	return cycles
}

// dedupCycles removes duplicate cycles that are rotations of each other.
// The key function maps each node to a stable string for comparison.
func dedupCycles[N any](cycles [][]N, key func(N) string) [][]N {
	seen := map[string]bool{}
	var out [][]N

	for _, c := range cycles {
		k := rotationKey(c, key)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	return out
}

// rotationKey produces a canonical string for a cycle, invariant under
// rotation. It picks the rotation starting at the node with the smallest
// key string.
func rotationKey[N any](cycle []N, key func(N) string) string {
	// ignore final repeated node for keying
	n := len(cycle) - 1

	minIdx := 0
	minKey := key(cycle[0])
	for i := 1; i < n; i++ {
		k := key(cycle[i])
		if k < minKey {
			minIdx = i
			minKey = k
		}
	}

	var b strings.Builder
	for i := range n {
		b.WriteString(key(cycle[(minIdx+i)%n]))
		b.WriteByte('>')
	}
	return b.String()
}

// ptrKey returns a pointer-based string key for any value.
func ptrKey[N any](n N) string {
	return fmt.Sprintf("%p", any(n))
}
