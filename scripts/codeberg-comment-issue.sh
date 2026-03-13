#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Add a comment to a Codeberg issue.
# Usage: codeberg-comment-issue.sh <api> <repo> <number> <body-file>
set -euo pipefail
: "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

api="$1"; repo="$2"; number="$3"; body_file="$4"

body=$(<"$body_file")

resp=$(curl -sf -X POST \
  -H "Authorization: token $CODEBERG_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --arg b "$body" '{body: $b}')" \
  "$api/repos/$repo/issues/$number/comments")
echo "$resp" | jq -r '"comment → \(.html_url)"'
