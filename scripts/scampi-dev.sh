#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Dev wrapper -- rebuilds if source changed, then runs scampi.
# Symlink this into ~/.local/bin/scampi for use anywhere.
set -euo pipefail

SRCDIR="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"
OUTDIR="$SRCDIR/build/bin"
BINPATH="$OUTDIR/scampi"

# Rebuild if binary is missing or any Go source file is newer.
# Hidden dirs (.git, .v1, .sandbox, ...) are pruned so unrelated
# activity in them doesn't trigger rebuilds here.
needs_build=false
if [[ ! -f "$BINPATH" ]]; then
  needs_build=true
else
  newer=$(find "$SRCDIR" -name '.*' -prune -o \
    -name '*.go' -newer "$BINPATH" -print 2>/dev/null | head -n 1)
  if [[ -n "$newer" ]]; then
    needs_build=true
  fi
fi

if [[ "$needs_build" == "true" ]]; then
  (cd "$SRCDIR" && just build) >&2
fi

exec "$BINPATH" "$@"
