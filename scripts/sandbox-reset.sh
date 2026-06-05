#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Wipe sandbox state without breaking tails: truncate JSONL segments
# in place and remove /tmp/scampi-* throwaway dirs.
set -euo pipefail

HERE="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"

shopt -s nullglob
for f in "$HERE"/.sandbox/log/*.jsonl; do
  : > "$f"
done

for d in /tmp/scampi-*; do
  rm -rf "$d"
done
