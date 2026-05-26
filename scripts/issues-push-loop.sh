#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Push loop: drains .issues/pusher-inbox/ to GitHub via `gh issue create`.
# Drafts arrive here pre-vetted (the eval-loop's job).
#
# File format (eval-loop guarantees this):
#   line 1: title
#   line 2: `labels: <l1>, <l2>, ...[, milestone: <title>]`
#   line 3: blank
#   line 4+: body markdown
#
# Files prefixed with _ are skipped (comment drafts).
#
# Per-file outcomes:
#   created on GitHub        -> rm draft, log issue URL, sleep CADENCE
#   server-side dup detected -> rm draft, log existing #N, sleep CADENCE
#   gh exits non-zero        -> rename to <draft>.failed, log stderr,
#                               sleep CADENCE (no infinite retry on
#                               broken drafts; eval-loop should have
#                               caught it)
#
# GitHub content-creation rate limits are generous (~80/min, 5000/hr
# authenticated) so the cadence is small but non-zero to keep the
# created-issues activity log readable.
#
# Log: .sandbox/push.log

set -u
cd "$(dirname "$0")/.." || exit 1

INBOX=".issues/pusher-inbox"
REPO="scampi-dev/scampi"
CADENCE=30

mkdir -p .sandbox

log() { echo "[$(date +%H:%M:%S)] $*" >> .sandbox/push.log; }

# Parse the draft's labels: line. Splits into label list + optional
# milestone title.
parse_meta() {
  local meta="$1"
  meta="${meta#labels:}"
  local labels=() milestone=""
  local IFS=','
  for raw in $meta; do
    # Trim surrounding whitespace.
    local item; item=$(printf '%s' "$raw" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')
    [[ -z "$item" ]] && continue
    if [[ "$item" == milestone:* ]]; then
      milestone="${item#milestone:}"
      milestone=$(printf '%s' "$milestone" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')
    else
      labels+=("$item")
    fi
  done
  printf '%s\n' "${labels[@]}"
  printf '__milestone__%s\n' "$milestone"
}

# Returns 0 if an open or closed issue with this exact title exists on
# the server. Logs the matched #N on hit.
title_exists_on_github() {
  local title="$1"
  local found
  found=$(gh issue list --repo "$REPO" --state all --search "\"$title\" in:title" \
    --json number,title --limit 20 2>/dev/null \
    | jq -r --arg t "$title" '.[] | select(.title == $t) | .number' \
    | head -1)
  if [[ -n "$found" ]]; then
    log "dup: '$title' already filed as #$found"
    return 0
  fi
  return 1
}

push_one() {
  local f="$1"
  local title labels_block body_tmp
  title=$(sed -n '1p' "$f")
  labels_block=$(sed -n '2p' "$f")

  if [[ -z "$title" ]]; then
    log "FAILED $f: empty title — renaming"
    mv "$f" "$f.failed"
    return
  fi
  if [[ "$labels_block" != labels:* ]]; then
    log "FAILED $f: line 2 is not 'labels:' — renaming"
    mv "$f" "$f.failed"
    return
  fi

  # Pre-flight dup check so we don't waste a creation attempt and don't
  # spam the timeline with "almost the same as #N" noise.
  if title_exists_on_github "$title"; then
    rm -f "$f"
    return
  fi

  # Body = lines 4+ (line 3 is the mandated blank separator).
  body_tmp=$(mktemp)
  trap 'rm -f "$body_tmp"' RETURN
  sed -n '4,$p' "$f" > "$body_tmp"

  local labels=() milestone=""
  while IFS= read -r line; do
    if [[ "$line" == __milestone__* ]]; then
      milestone="${line#__milestone__}"
    else
      labels+=("$line")
    fi
  done < <(parse_meta "$labels_block")

  local args=(--repo "$REPO" --title "$title" --body-file "$body_tmp")
  for l in "${labels[@]}"; do
    args+=(--label "$l")
  done
  if [[ -n "$milestone" ]]; then
    args+=(--milestone "$milestone")
  fi

  log "pushing '$title' labels=[${labels[*]}] milestone='$milestone'"
  local out rc
  out=$(gh issue create "${args[@]}" 2>&1)
  rc=$?
  if [[ $rc -eq 0 ]]; then
    log "created: $out"
    rm -f "$f"
  else
    log "FAILED $f rc=$rc: $(printf '%s' "$out" | tr '\n' ' ' | head -c 400)"
    mv "$f" "$f.failed"
  fi
}

log "push-loop start (watching $INBOX, cadence ${CADENCE}s)"

while true; do
  f=""
  [ -d "$INBOX" ] || { sleep 30; continue; }
  for candidate in "$INBOX"/*.md; do
    [ -e "$candidate" ] || continue
    base=$(basename "$candidate")
    case "$base" in
      _*) continue ;;
    esac
    f="$candidate"
    break
  done

  if [ -z "$f" ]; then
    sleep 30
    continue
  fi

  push_one "$f"
  sleep "$CADENCE"
done
