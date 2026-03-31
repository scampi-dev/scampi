#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Create Codeberg issues from markdown files.
# Usage: codeberg-create-issue.sh <api> <repo> <path> [--dry-run]
set -euo pipefail
: "${CODEBERG_TOKEN:?Set CODEBERG_TOKEN to a Codeberg personal access token}"

api="$1"; repo="$2"; path="$3"; shift 3

dry_run=false
for flag in "$@"; do
  case "$flag" in
    --dry-run) dry_run=true ;;
    *) echo "unknown flag: $flag"; exit 1 ;;
  esac
done

files=()
if [[ -d "$path" ]]; then
  for f in "$path"/*.md; do
    [[ -f "$f" ]] && files+=("$f")
  done
else
  files=("$path")
fi

if [[ ${#files[@]} -eq 0 ]]; then
  echo "No .md files found in $path"
  exit 1
fi

all_labels=$(curl -sf \
  -H "Authorization: token $CODEBERG_TOKEN" \
  "$api/repos/$repo/labels?limit=50")
existing_titles=$(curl -sf \
  -H "Authorization: token $CODEBERG_TOKEN" \
  "$api/repos/$repo/issues?type=issues&state=open&limit=50" \
  | jq -r '.[].title')

for f in "${files[@]}"; do
  title=$(sed -n '1p' "$f")
  labels_line=$(sed -n '2p' "$f")
  body=$(tail -n +4 "$f")

  label_ids="[]"
  label_names=""
  milestone_id=0
  if [[ "$labels_line" == labels:* ]]; then
    label_names="${labels_line#labels: }"
    label_ids=$(echo "$all_labels" | jq --arg names "$label_names" '
      ($names | split(", ")) as $want |
      [.[] | select(.name as $n | $want | any(. == $n)) | .id]')
  fi

  milestone_line=$(sed -n '3p' "$f")
  if [[ "$milestone_line" == milestone:* ]]; then
    milestone_id="${milestone_line#milestone: }"
    body=$(tail -n +5 "$f")
  fi

  if echo "$existing_titles" | grep -qxF "$title"; then
    echo "skip: $title (already exists)"
    continue
  fi

  if [[ "$dry_run" == true ]]; then
    echo "--- $f"
    echo "  title:     $title"
    echo "  labels:    $label_names"
    echo "  milestone: $milestone_id"
    echo "  body:      $(echo "$body" | wc -l | tr -d ' ') lines"
    continue
  fi

  resp=$(curl -sf \
    -H "Authorization: token $CODEBERG_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg t "$title" --arg b "$body" --argjson l "$label_ids" --argjson m "$milestone_id" \
      '{title: $t, body: $b, labels: $l, milestone: $m}')" \
    "$api/repos/$repo/issues")
  echo "$resp" | jq -r '"#\(.number): \(.title) → \(.html_url)"'

  if [[ ${#files[@]} -gt 1 ]]; then
    sleep 3
  fi
done
