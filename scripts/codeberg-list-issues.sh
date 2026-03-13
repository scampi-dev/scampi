#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# List open Codeberg issues.
# Usage: codeberg-list-issues.sh <api> <repo> [--label text]
set -euo pipefail
: "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

api="$1"; repo="$2"; shift 2

label=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --label) label="$2"; shift 2 ;;
    *) echo "unknown arg: $1"; exit 1 ;;
  esac
done

url="$api/repos/$repo/issues?type=issues&state=open&limit=50&sort=created&direction=asc"
if [[ -n "$label" ]]; then
  url="${url}&labels=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$label'))")"
fi

curl -sf \
  -H "Authorization: token $CODEBERG_TOKEN" \
  "$url" \
  | jq -r '.[] | "#\(.number)  \(.title)  [\(.labels | map(.name) | join(", "))]"'
