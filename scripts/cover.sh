#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Run the test suite with coverage tracking, aggregate per-package
# coverage, and print a table. -coverpkg=./internal/... so integration
# tests in internal/test/ count toward the packages they exercise.
#
# Each block (file:startline.col,endline.col) appears once per
# instrumented test binary; the awk pass dedupes so a block is counted
# once and its coverage is the OR across binaries.
set -euo pipefail

outdir="build/coverage"
profile="$outdir/coverage.out"
mkdir -p "$outdir"

go test -coverprofile="$profile" ./... >/dev/null

awk '
  NR == 1 { next }  # skip "mode: set"
  {
    block = $1
    if (!(block in stmts)) {
      stmts[block] = $2
      n = split($1, a, ":")
      file = a[1]
      sub(/\/[^\/]+$/, "", file)             # strip /<basename> to get pkg dir
      sub(/^scampi\.dev\/scampi\//, "", file) # trim module prefix
      pkg_of[block] = file
    }
    if ($3 > 0) covered[block] = 1
  }
  END {
    for (b in stmts) {
      p = pkg_of[b]
      total_pkg[p] += stmts[b]
      if (b in covered) cov_pkg[p] += stmts[b]
      total_all += stmts[b]
      if (b in covered) cov_all += stmts[b]
    }

    printf "PACKAGE\tSTMTS\tCOVERED\tCOVERAGE\n"

    # sort package names for stable output
    n_pkg = 0
    for (p in total_pkg) {
      n_pkg++
      names[n_pkg] = p
    }
    for (i = 1; i < n_pkg; i++) {
      for (j = i + 1; j <= n_pkg; j++) {
        if (names[i] > names[j]) { tmp = names[i]; names[i] = names[j]; names[j] = tmp }
      }
    }
    for (i = 1; i <= n_pkg; i++) {
      p = names[i]
      pct = total_pkg[p] > 0 ? (cov_pkg[p] / total_pkg[p] * 100) : 0
      printf "%s\t%d\t%d\t%.1f%%\n", p, total_pkg[p], cov_pkg[p], pct
    }

    pct_all = total_all > 0 ? (cov_all / total_all * 100) : 0
    printf "TOTAL\t%d\t%d\t%.1f%%\n", total_all, cov_all, pct_all
  }
' "$profile" | column -t -s $'\t'
