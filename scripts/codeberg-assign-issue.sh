#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Assign a Codeberg issue to a user.
# Usage: codeberg-assign-issue.sh <api> <repo> <number> <user>
set -euo pipefail
: "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

api="$1"; repo="$2"; number="$3"; user="$4"

resp=$(curl -sf -X PATCH \
  -H "Authorization: token $CODEBERG_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --arg u "$user" '{assignees: [$u]}')" \
  "$api/repos/$repo/issues/$number")
echo "$resp" | jq -r '"#\(.number): \(.title) → assigned to \(.assignees | map(.login) | join(", "))"'
