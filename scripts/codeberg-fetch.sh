#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Cached GET wrapper for the Codeberg API.
# Usage: codeberg-fetch.sh <url> [--refresh]
#
# Caches responses under $XDG_CACHE_HOME/scampi/codeberg/ with a 5-minute
# TTL. Pass --refresh to bypass the cache.
set -euo pipefail

url="$1"; shift
refresh=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --refresh) refresh=true; shift ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/scampi/codeberg"
cache_ttl=300  # 5 minutes
mkdir -p "$cache_dir"

# Derive a stable cache filename from the URL
cache_key=$(printf '%s' "$url" | shasum -a 256 | cut -d' ' -f1)
cache_file="$cache_dir/$cache_key"

if [[ "$refresh" == false && -f "$cache_file" ]]; then
  now=$(date +%s)
  if stat -f %m /dev/null &>/dev/null; then
    mtime=$(stat -f %m "$cache_file")
  else
    mtime=$(stat -c %Y "$cache_file")
  fi
  if (( now - mtime < cache_ttl )); then
    cat "$cache_file"
    exit 0
  fi
fi

curl_args=(-sf)
if [[ -n "${CODEBERG_TOKEN:-}" ]]; then
  curl_args+=(-H "Authorization: token $CODEBERG_TOKEN")
fi

json=$(curl "${curl_args[@]}" "$url")
printf '%s' "$json" > "$cache_file"
printf '%s' "$json"
