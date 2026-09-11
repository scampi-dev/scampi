#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Shared helpers for the dependency recipes. Sourced, not executed.

# Module directories we manage. site/ is a separate module: its only direct
# deps are the tool directives (hugo, gomarklint), everything else is
# indirect, so a direct-only scan there finds nothing at all.
# shellcheck disable=SC2034 # consumed by the scripts that source this file
GO_MODULE_DIRS=("." "site")

# upgrade_targets <dir>
#
# Prints every module in <dir>/go.mod that we upgrade deliberately, one per
# line: direct requires plus tool directives.
#
# Tool directives need the second pass because Go records them in require as
# "// indirect", so filtering indirect - which we must, or we would fight
# minimal version selection - also hides every tool. That is how
# golangci-lint sat three minor versions behind with no recipe able to move
# it.
upgrade_targets() {
  local dir=$1

  # Direct requires. Tolerate no matches: site/ has none at all, and grep
  # exits 1 on an empty result, which would be fatal under `set -e`.
  grep -E '^\t[^ ]+ v' "$dir/go.mod" | grep -v '// indirect' | awk '{print $1}' || true

  # Tool directives, block form and single-line form. The directive names a
  # package, which may sit below the module root (.../cmd/foo), so ask Go for
  # the owning module.
  {
    awk '/^tool \(/ {f=1; next} f && /^\)/ {f=0; next} f {print $1}' "$dir/go.mod"
    awk '/^tool [^(]/ {print $2}' "$dir/go.mod"
  } | while read -r pkg; do
    [ -n "$pkg" ] || continue
    go -C "$dir" list -f '{{.Module.Path}}' "$pkg" 2>/dev/null || true
  done
}
