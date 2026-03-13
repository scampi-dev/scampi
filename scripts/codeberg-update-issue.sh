#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Update a Codeberg issue.
# Usage: codeberg-update-issue.sh <api> <repo> <number> [--body file] [--title text] [--state text] [--labels text]
set -euo pipefail
: "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

api="$1"; repo="$2"; number="$3"; shift 3

body_file="" state="" labels="" title=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --body)   body_file="$2"; shift 2 ;;
    --state)  state="$2"; shift 2 ;;
    --labels) labels="$2"; shift 2 ;;
    --title)  title="$2"; shift 2 ;;
    *) echo "unknown arg: $1"; exit 1 ;;
  esac
done

patch='{}'

if [[ -n "$title" ]]; then
  patch=$(echo "$patch" | jq --arg t "$title" '. + {title: $t}')
fi

if [[ -n "$body_file" ]]; then
  body=$(<"$body_file")
  patch=$(echo "$patch" | jq --arg b "$body" '. + {body: $b}')
fi

if [[ -n "$state" ]]; then
  patch=$(echo "$patch" | jq --arg s "$state" '. + {state: $s}')
fi

if [[ "$patch" == '{}' && -z "$labels" ]]; then
  echo "nothing to update — pass --title, --body, --state, or --labels"
  exit 1
fi

if [[ "$patch" != '{}' ]]; then
  curl -sf -X PATCH \
    -H "Authorization: token $CODEBERG_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$patch" \
    "$api/repos/$repo/issues/$number" > /dev/null
fi

# Labels use a separate endpoint — PUT replaces all labels on the issue.
if [[ -n "$labels" ]]; then
  all_labels=$(curl -sf \
    -H "Authorization: token $CODEBERG_TOKEN" \
    "$api/repos/$repo/labels?limit=50")
  label_ids=$(echo "$all_labels" | jq --arg names "$labels" '
    ($names | split(", ")) as $want |
    [.[] | select(.name as $n | $want | any(. == $n)) | .id]')
  curl -sf -X PUT \
    -H "Authorization: token $CODEBERG_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --argjson l "$label_ids" '{labels: $l}')" \
    "$api/repos/$repo/issues/$number/labels" > /dev/null
fi

# Fetch final state for display
resp=$(curl -sf \
  -H "Authorization: token $CODEBERG_TOKEN" \
  "$api/repos/$repo/issues/$number")
echo "$resp" | jq -r '"#\(.number): \(.title) [\(.state)] → \(.html_url)"'
