#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Close a Codeberg milestone.
# Usage: codeberg-close-milestone.sh <api> <repo> <milestone_id>
set -euo pipefail
: "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

api="$1"; repo="$2"; milestone="$3"

curl -sf \
  -H "Authorization: token $CODEBERG_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"state": "closed"}' \
  -X PATCH \
  "$api/repos/$repo/milestones/$milestone" > /dev/null

echo "milestone $milestone closed"
