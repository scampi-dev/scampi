#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# List open Codeberg issues.
# Usage: codeberg-list-issues.sh <api> <repo> [--label text] [--milestone id] [--refresh]
set -euo pipefail
: "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

api="$1"; repo="$2"; shift 2
dir="$(cd "$(dirname "$0")" && pwd)"

label=""
milestone=""
refresh_flag=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --label) label="$2"; shift 2 ;;
    --milestone) milestone="$2"; shift 2 ;;
    --refresh) refresh_flag="--refresh"; shift ;;
    *) echo "unknown arg: $1"; exit 1 ;;
  esac
done

url="$api/repos/$repo/issues?type=issues&state=open&limit=50&sort=created&direction=asc"
if [[ -n "$label" ]]; then
  url="${url}&labels=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$label'))")"
fi
if [[ -n "$milestone" ]]; then
  url="${url}&milestones=${milestone}"
fi

"$dir/codeberg-fetch.sh" "$url" $refresh_flag \
  | jq -r '.[] | "#\(.number)  \(.title)  [\(.labels | map(.name) | join(", "))]"'
