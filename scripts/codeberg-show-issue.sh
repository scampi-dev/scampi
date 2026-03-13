#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Show full details for a Codeberg issue.
# Usage: codeberg-show-issue.sh <api> <repo> <number>
set -euo pipefail
: "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

api="$1"; repo="$2"; number="$3"

curl -sf \
  -H "Authorization: token $CODEBERG_TOKEN" \
  "$api/repos/$repo/issues/$number" \
  | jq -r '"#\(.number): \(.title)\nState: \(.state)\nLabels: \(.labels | map(.name) | join(", "))\nURL: \(.html_url)\n\n\(.body)"'

comments=$(curl -sf \
  -H "Authorization: token $CODEBERG_TOKEN" \
  "$api/repos/$repo/issues/$number/comments")

count=$(echo "$comments" | jq 'length')
if [[ "$count" -gt 0 ]]; then
  echo ""
  echo "--- comments ($count) ---"
  echo "$comments" | jq -r '.[] | "\n@\(.user.login) (\(.created_at)):\n\(.body)"'
fi
