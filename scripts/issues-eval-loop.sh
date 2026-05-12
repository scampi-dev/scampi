#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Eval loop: watches .issues/eval-inbox/ for new .md drafts. For each
# new file, invokes `claude -p` with the eval-prompt to verify format,
# normalize sibling refs, and check cited locations. On pass, claude
# moves the draft to .issues/pusher-inbox/ (the push-loop's queue).
# On fail, claude writes a .failed.txt sidecar.
#
# Skips files starting with _ (comment drafts not destined for push)
# and files ending in .failed.txt (already-rejected sidecars).
#
# Log: .sandbox/eval.log

set -u
cd "$(dirname "$0")/.." || exit 1

INBOX=".issues/eval-inbox"
PROMPT_FILE="scripts/issues-eval-prompt.md"
LOG=".sandbox/eval.log"

log() { echo "[$(date +%H:%M:%S)] $*" >> "$LOG"; }

eval_file() {
  local f="$1"
  local base; base=$(basename "$f")
  log "eval $f"

  # Substitute the file path into the prompt template via bash.
  local template prompt
  template=$(cat "$PROMPT_FILE")
  prompt="${template//\$EVAL_PATH/$f}"

  local out
  out=$(claude -p "$prompt" \
    --permission-mode acceptEdits \
    --no-session-persistence \
    --max-budget-usd 0.50 \
    --allowedTools "Read Edit Write Bash Glob Grep mcp__gopls__*" \
    2>&1)
  local rc=$?

  log "rc=$rc claude said: $(echo "$out" | tr '\n' ' ' | head -c 500)"
  if [[ -f "$f" ]] && [[ -f "${f}.failed.txt" ]]; then
    log "result: FAIL (sidecar at ${f}.failed.txt)"
  elif [[ ! -f "$f" ]] && [[ -f ".issues/pusher-inbox/$base" ]]; then
    log "result: PASS (moved to pusher-inbox)"
  else
    log "result: AMBIGUOUS — file still at $f, no sidecar"
  fi
}

log "eval-loop start"

while true; do
  found=""
  for f in "$INBOX"/*.md; do
    [[ -e "$f" ]] || continue
    base=$(basename "$f")
    case "$base" in
      _*|*.failed.txt) continue ;;
    esac
    found="$f"
    break
  done

  if [[ -z "$found" ]]; then
    sleep 30
    continue
  fi

  eval_file "$found"
  sleep 5
done
