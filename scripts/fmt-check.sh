#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Fail if any Go file would change under `just fmt`. Used by the
# lint pipeline so formatter drift never sneaks into a commit.
set -euo pipefail

drift=()

check() {
  local name="$1"
  shift
  local out
  out=$("$@")
  if [[ -n "$out" ]]; then
    drift+=("$name")
    printf '%s drift:\n' "$name"
    printf '  %s\n' "$out"
  fi
}

check gofumpt go tool gofumpt -l .
check golines go tool golines -m 100 -l .

if [[ ${#drift[@]} -gt 0 ]]; then
  echo "Run 'just fmt' to fix."
  exit 1
fi
