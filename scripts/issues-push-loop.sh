#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Push loop: drains .issues/pusher-inbox/ to Codeberg.
# Drafts arrive here pre-vetted (the eval-loop's job).
#
# Files prefixed with _ are skipped (comment drafts).
#
# Exit codes from `just cb create-issue` (see codeberg-create-issue.sh):
#   0 = created or skipped-as-duplicate
#   2 = permanent failure → rename .failed
#   3 = rate-limited (429) → back off 5min, retry same file
#   4 = transient/uncertain (5xx) → re-fetch existing titles and verify
#       whether the create succeeded server-side before retrying
#
# Log: .sandbox/push.log

set -u
cd "$(dirname "$0")/.." || exit 1

INBOX=".issues/pusher-inbox"
CODEBERG_API="https://codeberg.org/api/v1"
CODEBERG_REPO="scampi-dev/scampi"

log() { echo "[$(date +%H:%M:%S)] $*" >> .sandbox/push.log; }

title_exists_on_codeberg() {
  local title="$1"
  local body
  body=$(curl -s -H "Authorization: token ${CODEBERG_TOKEN}" \
    "${CODEBERG_API}/repos/${CODEBERG_REPO}/issues?type=issues&state=open&limit=50&sort=created&direction=desc")
  echo "$body" | jq -r '.[].title' | grep -qxF "$title"
}

log "push-loop start (watching $INBOX, cadence 75s, rate-limit backoff 300s)"

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

  log "pushing $f"
  out=$(just cb create-issue "$f" 2>&1)
  rc=$?
  log "rc=$rc out=$out"

  case $rc in
    0)
      sleep 75
      ;;
    3)
      log "rate-limited on $f — backoff 300s, will retry"
      sleep 300
      ;;
    4)
      title=$(sed -n '1p' "$f")
      log "5xx on $f — verifying whether '$title' was created server-side"
      sleep 5
      if title_exists_on_codeberg "$title"; then
        log "verified server-side success for $f — removing draft"
        rm -f "$f"
        sleep 75
      else
        log "not yet on server — backoff 120s, retry"
        sleep 120
      fi
      ;;
    2)
      log "FAILED $f rc=2 (permanent) — renaming to $f.failed"
      mv "$f" "$f.failed"
      sleep 75
      ;;
    *)
      log "FAILED $f rc=$rc (unexpected) — renaming to $f.failed"
      mv "$f" "$f.failed"
      sleep 75
      ;;
  esac
done
