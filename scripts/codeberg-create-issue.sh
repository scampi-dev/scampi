#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Create Codeberg issues from markdown files.
# Usage: codeberg-create-issue.sh <api> <repo> <path> [--dry-run]
#
# On successful create (201) or server-reported duplicate, the source
# markdown file is deleted — the file is a queue entry, the issue is
# the persistent artifact. --dry-run never deletes.
#
# Exit codes:
#   0   success (issue created, or skipped as duplicate)
#   1   bad usage / no files
#   2   permanent failure (4xx other than 429, malformed config, etc.)
#   3   rate-limited (429) — caller should back off and retry
#   4   transient/uncertain (5xx including 504) — issue may or may not
#       have been created; caller should re-fetch existing titles before
#       retrying to avoid duplicates
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
    [[ "$dry_run" == false ]] && rm -f "$f"
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

  # Two-step create: POST issue without labels (fast), then POST labels
  # to /issues/{number}/labels. Codeberg's create-with-labels path
  # sometimes 504s at the gateway while the no-labels path succeeds
  # instantly; the dedicated labels endpoint is also fast. Two
  # requests, lower failure rate.
  body_file=$(mktemp)
  trap 'rm -f "$body_file"' EXIT
  status=$(curl -s -o "$body_file" -w '%{http_code}' \
    -H "Authorization: token $CODEBERG_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg t "$title" --arg b "$body" --argjson m "$milestone_id" \
      '{title: $t, body: $b, labels: [], milestone: $m}')" \
    "$api/repos/$repo/issues")

  case "$status" in
    201)
      issue_number=$(jq -r '.number' "$body_file")
      jq -r '"#\(.number): \(.title) → \(.html_url)"' "$body_file"
      if [[ "$label_ids" != "[]" ]]; then
        label_status=$(curl -s -o /dev/null -w '%{http_code}' \
          -H "Authorization: token $CODEBERG_TOKEN" \
          -H "Content-Type: application/json" \
          -d "$(jq -n --argjson l "$label_ids" '{labels: $l}')" \
          "$api/repos/$repo/issues/$issue_number/labels")
        if [[ "$label_status" != "200" ]]; then
          echo "  warning: label attach returned $label_status — issue created but unlabeled" >&2
        fi
      fi
      rm -f "$f"
      ;;
    409|422)
      # Codeberg returns 409/422 for duplicate or validation errors;
      # treat duplicate-title as skip, other 422s as permanent.
      msg=$(jq -r '.message // .' "$body_file" 2>/dev/null || cat "$body_file")
      if echo "$msg" | grep -qi 'already exists\|duplicate'; then
        echo "skip: $title (server reports already exists)"
        rm -f "$f"
      else
        echo "permanent failure ($status): $title — $msg" >&2
        rm -f "$body_file"
        exit 2
      fi
      ;;
    429)
      retry=$(curl -s -I -H "Authorization: token $CODEBERG_TOKEN" \
        "$api/repos/$repo/issues" | grep -i 'retry-after:' | tr -d '\r' || true)
      echo "rate-limited (429) on: $title ${retry:+(${retry})}" >&2
      rm -f "$body_file"
      exit 3
      ;;
    5*)
      # 5xx: backend may or may not have created the issue. The caller
      # is responsible for re-fetching titles before retrying. We exit
      # 4 to signal "uncertain — verify before retry".
      msg=$(head -c 500 "$body_file" 2>/dev/null || true)
      echo "transient ($status) on: $title — issue may have been created; verify before retry" >&2
      [[ -n "$msg" ]] && echo "  body: $msg" >&2
      rm -f "$body_file"
      exit 4
      ;;
    *)
      msg=$(jq -r '.message // .' "$body_file" 2>/dev/null || cat "$body_file")
      echo "unexpected HTTP $status on: $title — $msg" >&2
      rm -f "$body_file"
      exit 2
      ;;
  esac
  rm -f "$body_file"

  if [[ ${#files[@]} -gt 1 ]]; then
    sleep 3
  fi
done
