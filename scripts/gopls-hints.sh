#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# gopls check prints diagnostics but exits 0; turn any output into a lint failure.
set -euo pipefail

files=$(git ls-files --cached --others --exclude-standard -- '*.go')
[[ -z "$files" ]] && exit 0
out=$(echo "$files" | xargs go tool gopls check -severity=hint 2>&1)
[[ -z "$out" ]] || { echo "$out"; exit 1; }
