#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Shared dev runner — rebuilds a binary if Go source changed, then execs it.
# Usage: dev-run.sh <binary> [args...]
#   binary: "scampi" or "scampls"
set -euo pipefail

BIN="$1"; shift
SRCDIR="$(cd "$(dirname "$0")/.." && pwd)"
OUTDIR="$SRCDIR/build/bin"
BINPATH="$OUTDIR/$BIN"

# Rebuild if binary is missing or any .go file is newer.
needs_build=false
if [[ ! -f "$BINPATH" ]]; then
  needs_build=true
else
  # Find any .go file newer than the binary.
  if [[ -n "$(find "$SRCDIR" -name '*.go' -newer "$BINPATH" -print -quit 2>/dev/null)" ]]; then
    needs_build=true
  fi
fi

if [[ "$needs_build" == "true" ]]; then
  version="$(cd "$SRCDIR" && git describe --tags --always --dirty 2>/dev/null || echo dev)"
  ldflags="-s -w -X main.version=$version"
  mkdir -p "$OUTDIR"
  (cd "$SRCDIR" && go build -ldflags "$ldflags" -o "$OUTDIR/$BIN" "./cmd/$BIN") >&2
fi

exec "$BINPATH" "$@"
