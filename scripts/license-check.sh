#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
set -euo pipefail

spdx="SPDX-License-Identifier: GPL-3.0-only"
missing=()
wrong=()
stray=()

check_file() {
  local f="$1" line="$2"
  local actual
  actual=$(sed -n "${line}p" "$f")
  if [[ "$actual" != *"$spdx"* ]]; then
    if grep -q 'SPDX-License-Identifier' "$f"; then
      wrong+=("$f  (line ${line}: $actual)")
    else
      missing+=("$f")
    fi
  fi
  local count
  count=$(grep -c 'SPDX-License-Identifier' "$f" || true)
  if [[ "$count" -gt 1 ]]; then
    stray+=("$f  (${count} occurrences)")
  fi
}

# Restrict to tracked + untracked-not-ignored files so anything in
# .gitignore (.sandbox/, build/, etc.) is naturally skipped.
# .dev/ scripts are tracked but exempt from the SPDX requirement,
# so they're filtered out explicitly below.
list_tracked() {
  local pattern="$1"
  git ls-files --cached --others --exclude-standard -- "$pattern" \
    | grep -v '^\.dev/' || true
}

# Go files: SPDX on line 1. Generated files (stringer, mockgen, etc.)
# get their SPDX header via scripts/spdx-stamp.sh chained from
# `just generate`, so they're checked like any other Go file.
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  check_file "$f" 1
done < <(list_tracked '*.go')

# Shell scripts: SPDX on line 2 (after shebang)
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  [[ "$(basename "$f")" == "license-check.sh" ]] && continue
  check_file "$f" 2
done < <(list_tracked '*.sh')

ok=true
if [[ ${#missing[@]} -gt 0 ]]; then
  echo "MISSING SPDX header:"
  printf '  %s\n' "${missing[@]}"
  ok=false
fi
if [[ ${#wrong[@]} -gt 0 ]]; then
  echo "WRONG line for SPDX header:"
  printf '  %s\n' "${wrong[@]}"
  ok=false
fi
if [[ ${#stray[@]} -gt 0 ]]; then
  echo "DUPLICATE SPDX headers:"
  printf '  %s\n' "${stray[@]}"
  ok=false
fi
if [[ "$ok" == true ]]; then
  n_go=$(list_tracked '*.go' | grep -cv '^$' || true)
  n_sh=$(list_tracked '*.sh' | grep -v '^$' | grep -cv '/license-check\.sh$' || true)
  echo "OK: all $((n_go + n_sh)) files have correct SPDX headers"
fi
[[ "$ok" == true ]]
