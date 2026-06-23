#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Dev wrapper — rebuilds scampi if Go source (or an embedded std/ stub)
# changed, then execs it. Symlink into ~/.local/bin/scampi to use anywhere.
set -euo pipefail

SRCDIR="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"
OUTDIR="$SRCDIR/build/bin"
BINPATH="$OUTDIR/scampi"

# Rebuild if the binary is missing or any source file is newer. Includes
# .go files and the .scampi stubs under std/ — those are embedded at
# compile time, so editing a stub needs a rebuild to take effect.
needs_build=false
if [[ ! -f "$BINPATH" ]]; then
  needs_build=true
else
  newer=$(find "$SRCDIR" \
    \( -name '*.go' -o -path "$SRCDIR/std/*.scampi" \) \
    -newer "$BINPATH" -print -quit 2>/dev/null)
  [[ -n "$newer" ]] && needs_build=true
fi

if [[ "$needs_build" == "true" ]]; then
  version="$(cd "$SRCDIR" && git describe --tags --always --dirty 2>/dev/null || echo dev)"
  mkdir -p "$OUTDIR"
  (cd "$SRCDIR" && go build -ldflags "-s -w -X main.version=$version" -o "$BINPATH" ./cmd/scampi) >&2
fi

exec "$BINPATH" "$@"
