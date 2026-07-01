#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
set -euo pipefail

spdx="SPDX-License-Identifier: GPL-3.0-only"

# One git query for everything tracked + untracked-not-ignored, so anything in
# .gitignore — .sandbox/, .issues/, build/, etc. — is naturally skipped. Filter
# per file type in-shell from here on; never shell out to git again. .dev/
# scripts and testdata fixtures are tracked but exempt, filtered below.
all_files=$(git ls-files --cached --others --exclude-standard | grep -v '^\.dev/' || true)

pick() { printf '%s\n' "$all_files" | grep -E "$1" || true; }

# Go + scampi: SPDX on line 1 (scampi testdata fixtures exempt).
go_files=$(pick '\.go$')
scampi_files=$(pick '\.scampi$' | grep -v '/testdata/' || true)
# Shell + Python scripts: SPDX on line 2, after the shebang (this script exempt).
sh_files=$(pick '\.sh$' | grep -v '/license-check\.sh$' || true)
py_files=$(pick '\.py$')

# scan checks every file in a group in a single awk process (vs sed+grep per
# file, which spawns ~2 subprocesses each and dominates the runtime). The
# newline-separated list arrives on stdin; awk verifies the SPDX header appears
# exactly once, on the required line, and emits "<kind>\t<file>\t<detail>" for
# each offender. reqline is the line the identifier must sit on.
scan() {
  local reqline="$1" f
  local files=()
  while IFS= read -r f; do [[ -n "$f" ]] && files+=("$f"); done
  [[ ${#files[@]} -eq 0 ]] && return 0
  awk -v reqline="$reqline" -v spdx="$spdx" '
    FNR==1 { if (seen) emit(); seen=1; file=FILENAME; count=0; onreq=0; reqtext="" }
    /SPDX-License-Identifier/ { count++ }
    FNR==reqline { reqtext=$0; if (index($0, spdx)) onreq=1 }
    END { if (seen) emit() }
    function emit() {
      if (!onreq) {
        if (count==0) print "missing\t" file "\t"
        else          print "wrong\t"   file "\t" reqtext
      }
      if (count>1)    print "stray\t"   file "\t" count
    }
  ' "${files[@]}"
}

results=$(
  printf '%s\n' "$go_files" "$scampi_files" | scan 1
  printf '%s\n' "$sh_files" "$py_files"     | scan 2
)

missing=()
wrong=()
stray=()
while IFS=$'\t' read -r kind file detail; do
  case "$kind" in
    missing) missing+=("$file") ;;
    wrong)   wrong+=("$file  (line: $detail)") ;;
    stray)   stray+=("$file  ($detail occurrences)") ;;
  esac
done <<< "$results"

ok=true
if [[ ${#missing[@]} -gt 0 ]]; then
  echo "✗ Missing SPDX header:"
  printf '  %s\n' "${missing[@]}"
  ok=false
fi
if [[ ${#wrong[@]} -gt 0 ]]; then
  echo "✗ SPDX header present but on the wrong line:"
  printf '  %s\n' "${wrong[@]}"
  ok=false
fi
if [[ ${#stray[@]} -gt 0 ]]; then
  echo "✗ Duplicate SPDX headers:"
  printf '  %s\n' "${stray[@]}"
  ok=false
fi
if [[ "$ok" == true ]]; then
  count() { printf '%s\n' "$1" | grep -cv '^$' || true; }
  total=$(( $(count "$go_files") + $(count "$scampi_files") \
          + $(count "$sh_files") + $(count "$py_files") ))
  echo "✓ All ${total} files have correct SPDX headers"
fi
[[ "$ok" == true ]]
