#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Wipe sandbox state without breaking tails: truncate JSONL segments
# in place and remove /tmp/scampi-* throwaway dirs.
set -euo pipefail

HERE="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"

shopt -s nullglob globstar
for f in "$HERE"/.sandbox/**/0001.jsonl; do
  : > "$f"
done
for f in "$HERE"/.sandbox/**/peers.json; do
  rm -f "$f"
done

for d in /tmp/scampi-*; do
  rm -rf "$d"
done
