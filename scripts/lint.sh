#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Run every linter. Each prints a one-line "<name> ... OK" on success
# (output suppressed). On failure, prints "<name> ... FAIL" followed
# by the captured output, and the script exits non-zero. All linters
# run regardless of intermediate failures so a single run surfaces
# everything that needs fixing.
set -uo pipefail
shopt -s globstar

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

section golangci-lint go tool golangci-lint run
section gomarklint    go tool gomarklint
section gopls-hints   ./scripts/gopls-hints.sh
section shellcheck    shellcheck ./**/*.sh
section license-check ./scripts/license-check.sh
section fmt-check     ./scripts/fmt-check.sh

exit "$fail"
