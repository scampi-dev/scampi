#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
set -euo pipefail
direct=$(grep -E '^\t[^ ]+ v' go.mod \
  | grep -v '// indirect' | awk '{print $1}')
# shellcheck disable=SC2086 # intentional word splitting
mods=$(go list -m -u $direct 2>/dev/null \
  | grep '\[' | awk '{print $1 "@latest"}')
if [[ -z "$mods" ]]; then
  echo "All direct dependencies are up to date."
  exit 0
fi
echo "$mods" | xargs go get
go mod tidy
