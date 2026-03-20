#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
set -euo pipefail
direct=$(grep -E '^\t[^ ]+ v' go.mod \
  | grep -v '// indirect' | awk '{print $1}')
# shellcheck disable=SC2086 # intentional word splitting
go list -m -u $direct 2>/dev/null | grep '\[' || true
