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

# Go files: SPDX on line 1
while IFS= read -r f; do
  check_file "$f" 1
done < <(find . -name '*.go' -not -path './vendor/*')

# Shell scripts: SPDX on line 2 (after shebang)
while IFS= read -r f; do
  check_file "$f" 2
done < <(find . -name '*.sh' -not -path './vendor/*' -not -path './build/*' -not -path './.dev/*' -not -name 'license-check.sh')

# Scampi-lang files: SPDX on line 1
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  check_file "$f" 1
done < <(find . -name '*.scampi' -not -path './.sandbox/*' -not -path '*/testdata/*')

ok=true
if [[ ${#missing[@]} -gt 0 ]]; then
  echo "✗ Missing SPDX header:"
  printf '  %s\n' "${missing[@]}"
  ok=false
fi
if [[ ${#wrong[@]} -gt 0 ]]; then
  echo "✗ SPDX header present but not on line 1:"
  printf '  %s\n' "${wrong[@]}"
  ok=false
fi
if [[ ${#stray[@]} -gt 0 ]]; then
  echo "✗ Duplicate SPDX headers:"
  printf '  %s\n' "${stray[@]}"
  ok=false
fi
if [[ "$ok" == true ]]; then
  n_go=$(find . -name '*.go' -not -path './vendor/*' | wc -l)
  n_sh=$(find . -name '*.sh' -not -path './vendor/*' -not -path './build/*' -not -path './site/static/*' -not -path './.dev/*' | wc -l)
  n_scampi=$(find . -name '*.scampi' -not -path './.sandbox/*' -not -path '*/testdata/*' | wc -l)
  echo "✓ All $((n_go + n_sh + n_scampi)) files have correct SPDX headers"
fi
[[ "$ok" == true ]]
