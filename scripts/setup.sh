#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Install build/test/lint tooling. Each subsystem prints a one-line
# OK/FAIL so a partial failure (e.g. go tools install but an OS tool
# is missing) is unambiguous.
#
# Usage: setup.sh <required-tool>...
#   Required OS tools are passed as positional arguments by the
#   justfile so the canonical list lives there, not duplicated here.
#
# OS tools install is NOT captured: when packages actually need to be
# installed we want sudo prompts and package-manager progress visible.
set -uo pipefail

fail=0

section() {
  local name="$1"
  shift
  local out
  if out=$("$@" 2>&1); then
    printf '%-15s ... OK\n' "$name"
    return 0
  fi
  printf '%-15s ... FAIL\n' "$name"
  printf '  %s\n' "${out//$'\n'/$'\n'  }"
  fail=1
}

section "go download" go mod download
section "go tidy"     go mod tidy

# OS tools: check first; if everything's present, OK and done.
# If anything's missing, install with visible output so sudo prompts
# and apt-get progress are not hidden.
missing=()
for cmd in "$@"; do
  command -v "$cmd" &>/dev/null || missing+=("$cmd")
done

if [[ ${#missing[@]} -eq 0 ]]; then
  printf '%-15s ... OK\n' "os tools"
else
  printf '%-15s ... installing %s\n' "os tools" "${missing[*]}"
  if command -v brew &>/dev/null; then
    brew install "${missing[@]}" || fail=1
  elif command -v pacman &>/dev/null; then
    sudo pacman -S "${missing[@]}" || fail=1
  elif command -v dnf &>/dev/null; then
    sudo dnf install -y "${missing[@]}" || fail=1
  elif command -v apt-get &>/dev/null; then
    sudo apt-get install -y "${missing[@]}" || fail=1
  else
    echo "  install manually: ${missing[*]}" >&2
    fail=1
  fi
fi

exit "$fail"
