#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Binary size analysis for scampi and scampls.
# Builds unstripped binaries to a temp dir, then reports per-module sizes.
set -euo pipefail

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

bold=$'\033[1m'
dim=$'\033[2m'
reset=$'\033[0m'

printf '%s\n' "${bold}Building unstripped binaries…${reset}"
go build -o "$tmpdir/scampi"  ./cmd/scampi
go build -o "$tmpdir/scampls" ./cmd/scampls

# Also build stripped for comparison
ldflags="-s -w"
go build -ldflags "$ldflags" -o "$tmpdir/scampi_stripped"  ./cmd/scampi
go build -ldflags "$ldflags" -o "$tmpdir/scampls_stripped" ./cmd/scampls

printf '\n%s\n' "${bold}On-disk sizes${reset}"
printf "  %-12s %8s  %8s\n" "" "debug" "stripped"
for bin in scampi scampls; do
  debug=$(stat -f%z "$tmpdir/$bin" 2>/dev/null || stat -c%s "$tmpdir/$bin")
  stripped=$(stat -f%z "$tmpdir/${bin}_stripped" 2>/dev/null || stat -c%s "$tmpdir/${bin}_stripped")
  printf "  %-12s %7sM  %7sM\n" "$bin" \
    "$(echo "scale=1; $debug/1048576" | bc)" \
    "$(echo "scale=1; $stripped/1048576" | bc)"
done

# Mach-O segment breakdown (macOS) or ELF section breakdown (Linux)
for bin in scampi scampls; do
  printf '\n%s\n' "${bold}Segment sizes: ${bin} (stripped)${reset}"
  if command -v otool &>/dev/null; then
    otool -l "$tmpdir/${bin}_stripped" | awk '
      /cmd LC_SEGMENT_64/ { seg=1 }
      seg && /segname/    { name=$2 }
      seg && /filesize/   { if ($2+0 > 0) printf "  %-16s %7.1fM\n", name, $2/1048576; seg=0 }
    '
  elif command -v readelf &>/dev/null; then
    readelf -S "$tmpdir/${bin}_stripped" | awk '
      /\[/ && NF>5 { sizes[$2] = strtonum("0x"$6) }
      END {
        n=0; for(s in sizes){keys[n]=s; vals[n]=sizes[s]; n++}
        for(i=1;i<n;i++){k=keys[i];v=vals[i];j=i-1; while(j>=0&&vals[j]<v){keys[j+1]=keys[j];vals[j+1]=vals[j];j--}; keys[j+1]=k;vals[j+1]=v}
        limit=(n<10)?n:10; for(i=0;i<limit;i++) printf "  %-16s %7.1fM\n", keys[i], vals[i]/1048576
      }
    '
  fi
done

# Per-module size breakdown (text+rodata+data, excludes BSS)
nm_breakdown() {
  local binary="$1" label="$2"
  printf '\n%s\n' "${bold}Per-module sizes: ${label}${reset}  ${dim}(text+rodata+data, excludes BSS)${reset}"
  go tool nm -size "$binary" | awk '
    NF >= 4 && $2+0 > 0 && $3 !~ /^[BbU]$/ {
      size = $2 + 0
      name = $4
      n = split(name, parts, ".")
      pkg = ""
      for (i = 1; i < n; i++) {
        if (i > 1) pkg = pkg "."
        pkg = pkg parts[i]
      }
      if (pkg == "") pkg = name
      gsub(/\.\(\*[^)]*\)/, "", pkg)

      # Group to top-level module
      if (pkg ~ /^scampi\.dev/) {
        split(pkg, s, "/"); mod = s[1]"/"s[2]"/"s[3]
        if (length(s) >= 4) mod = mod "/" s[4]
      } else if (pkg ~ /^github\.com|^golang\.org|^filippo\.io|^go\.lsp\.dev|^go\.uber\.org|^gopkg\.in/) {
        split(pkg, s, "/"); mod = s[1]"/"s[2]"/"s[3]
      } else if (pkg ~ /^vendor\//) {
        split(pkg, s, "/"); mod = s[1]"/"s[2]"/"s[3]"/"s[4]
      } else {
        split(pkg, s, "/"); mod = s[1]
      }
      totals[mod] += size
    }
    END {
      n = 0
      for (p in totals) { keys[n] = p; vals[n] = totals[p]; n++ }
      # Insertion sort descending by size
      for (i = 1; i < n; i++) {
        k = keys[i]; v = vals[i]; j = i - 1
        while (j >= 0 && vals[j] < v) {
          keys[j+1] = keys[j]; vals[j+1] = vals[j]; j--
        }
        keys[j+1] = k; vals[j+1] = v
      }
      limit = (n < 40) ? n : 40
      for (i = 0; i < limit; i++) {
        kb = vals[i] / 1024
        if (kb >= 1024)
          printf "  %6.1fM  %s\n", kb/1024, keys[i]
        else
          printf "  %5.0fK  %s\n", kb, keys[i]
      }
    }
  '
}

nm_breakdown "$tmpdir/scampi"  "scampi"
nm_breakdown "$tmpdir/scampls" "scampls"

# Dependency count
printf '\n%s\n' "${bold}Package counts${reset}"
printf "  scampi:  %d packages\n" "$(go list -deps ./cmd/scampi  2>/dev/null | wc -l | tr -d ' ')"
printf "  scampls: %d packages\n" "$(go list -deps ./cmd/scampls 2>/dev/null | wc -l | tr -d ' ')"

# Heavy dep import chains
printf '\n%s\n' "${bold}Notable import chains${reset}"
for dep in "filippo.io/age" "github.com/getkin/kin-openapi" "github.com/klauspost/compress" "github.com/pkg/sftp"; do
  importers=$(go list -deps -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}' ./cmd/scampi 2>/dev/null \
    | grep "scampi\.dev.*$dep" | awk '{print $1}')
  if [[ -n "$importers" ]]; then
    printf '  %s\n' "${dim}${dep}${reset} ← $(echo "$importers" | tr '\n' ', ' | sed 's/,$//')"
  fi
done

# scampls-specific: what shouldn't be there?
printf '\n%s\n' "${bold}Packages in scampls that may be unnecessary${reset}"
go list -deps ./cmd/scampls 2>/dev/null | grep -E 'filippo\.io/age|step/|target/' | while read -r pkg; do
  printf "  %s\n" "$pkg"
done
