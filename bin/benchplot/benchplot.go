// SPDX-License-Identifier: GPL-3.0-only

// benchplot.go
//
// Usage:
//   go run benchplot.go ../../benchmarks/*.txt > bench.csv
//
// Output CSV:
//   benchmark,timestamp,median_ns
//   ApplyNoOp,2026-01-13T13:49,13811482
//   ApplyNoOp,2026-01-13T21:47,14072509
//   ApplyNoOp,2026-01-14T09:41,7981038
//   ApplyNoOp,2026-01-14T16:12,8210382
//   DiagnosticEmission-14,2026-01-13T13:49,214
//   DiagnosticEmission-14,2026-01-13T21:47,216
//   DiagnosticEmission-14,2026-01-14T09:41,222
//   DiagnosticEmission-14,2026-01-14T16:12,211
//   LoadConfig,2026-01-13T13:49,8965372
//   LoadConfig,2026-01-13T21:47,9063043
//   LoadConfig,2026-01-14T09:41,9125790
//   LoadConfig,2026-01-14T16:12,9476692

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

var (
	benchRe = regexp.MustCompile(
		`^Benchmark([^/\s]+)(?:/Size-\d+-\d+)?\s+\d+\s+([\d.]+)\s+ns/op`,
	)

	tsRe = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})T(\d{2})(\d{2})`)
)

type key struct {
	name string
	ts   string
}

func main() {
	if len(os.Args) < 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: benchplot.go <benchmark files>")
		os.Exit(1)
	}

	values := make(map[key][]float64)

	for _, path := range os.Args[1:] {
		ts := extractTimestamp(path)
		if ts == "" {
			continue
		}
		parseFile(path, ts, values)
	}

	_, _ = fmt.Println("benchmark,timestamp,median_ms")

	// stable output
	var keys []key
	for k := range values {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name != keys[j].name {
			return keys[i].name < keys[j].name
		}
		return keys[i].ts < keys[j].ts
	})

	for _, k := range keys {
		med := median(values[k])
		_, _ = fmt.Printf("%s,%s,%.0f\n", k.name, k.ts, med/1000000.0)
	}
}

func extractTimestamp(path string) string {
	base := filepath.Base(path)
	m := tsRe.FindStringSubmatch(base)
	if m == nil {
		return ""
	}
	return fmt.Sprintf("%sT%s:%s", m[1], m[2], m[3])
}

func parseFile(path, ts string, out map[key][]float64) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		m := benchRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		name := m[1]
		ns, _ := strconv.ParseFloat(m[2], 64)

		k := key{name: name, ts: ts}
		out[k] = append(out[k], ns)
	}
}

func median(xs []float64) float64 {
	slices.Sort(xs)
	n := len(xs)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return xs[n/2]
	}
	return (xs[n/2-1] + xs[n/2]) / 2
}
