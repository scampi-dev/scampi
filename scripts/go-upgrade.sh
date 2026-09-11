#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source=scripts/go-modules.sh
source "$root/scripts/go-modules.sh"
cd "$root"

# NOTE: always `go get mod@latest`, never `go get -u`. The -u flag also
# upgrades a module's transitive deps past the versions it was built
# against, which breaks tools that vendor a pinned analysis tree -
# golangci-lint fails with `undefined: glob.Glob` that way.
#
# NOTE: site/ is a hybrid module - Go tool deps (hugo, gomarklint) plus Hugo
# module deps (the hextra theme, declared in hugo.toml). No Go file imports
# the theme, so `go mod tidy` prunes it; conversely `hugo mod tidy` drops the
# whole Go tool tree. Neither is correct alone, so we run `go mod tidy` and
# then let Hugo restore its own modules.

upgraded=0
for dir in "${GO_MODULE_DIRS[@]}"; do
  mapfile -t targets < <(upgrade_targets "$dir" | sort -u)
  [ "${#targets[@]}" -gt 0 ] || continue

  mapfile -t stale < <(
    go -C "$dir" list -m -u "${targets[@]}" 2>/dev/null \
      | grep '\[' | awk '{print $1 "@latest"}'
  )
  [ "${#stale[@]}" -gt 0 ] || continue

  echo "$dir:"
  printf '  %s\n' "${stale[@]}"
  go -C "$dir" get "${stale[@]}"
  go -C "$dir" mod tidy

  # Restore Hugo module deps that `go mod tidy` just pruned (see NOTE above),
  # so an upgrade never leaves the tree dirty.
  if printf '%s\n' "${targets[@]}" | grep -qx 'github.com/gohugoio/hugo'; then
    go -C "$dir" tool github.com/gohugoio/hugo mod get >/dev/null
  fi

  upgraded=1
done

if [ "$upgraded" -eq 0 ]; then
  echo "All managed dependencies are up to date."
fi
