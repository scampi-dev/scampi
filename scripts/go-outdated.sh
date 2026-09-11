#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source=scripts/go-modules.sh
source "$root/scripts/go-modules.sh"
cd "$root"

found=0
for dir in "${GO_MODULE_DIRS[@]}"; do
  mapfile -t targets < <(upgrade_targets "$dir" | sort -u)
  [ "${#targets[@]}" -gt 0 ] || continue

  out=$(go -C "$dir" list -m -u "${targets[@]}" 2>/dev/null | grep '\[' || true)
  [ -n "$out" ] || continue

  found=1
  printf '%s\n' "$dir:"
  printf '%s\n' "$out" | sed 's/^/  /'
done

if [ "$found" -eq 0 ]; then
  echo "All managed dependencies are up to date."
fi
