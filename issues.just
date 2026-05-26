# scampi issue pipeline -- two loops:
#   eval  (.issues/eval-inbox/  -> claude-code QA -> .issues/pusher-inbox/)
#   push  (.issues/pusher-inbox/ -> GitHub via `gh issue create`)

# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list issues {{args}}

# Control
# -----------------------------------------------------------------------------

[group('control')]
[doc("Start both loops")]
up:
  #!/usr/bin/env bash
  set -u
  if pgrep -f 'scripts/issues-eval-loop.sh' >/dev/null; then
    echo "eval-loop already running"
  else
    nohup bash scripts/issues-eval-loop.sh >/dev/null 2>&1 &
    echo "eval-loop started (pid $!)"
  fi
  if pgrep -f 'scripts/issues-push-loop.sh' >/dev/null; then
    echo "push-loop already running"
  else
    nohup bash scripts/issues-push-loop.sh >/dev/null 2>&1 &
    echo "push-loop started (pid $!)"
  fi

[group('control')]
[doc("Stop both loops")]
down:
  @pkill -f 'scripts/issues-eval-loop.sh' && echo "eval-loop stopped" || echo "eval-loop not running"
  @pkill -f 'scripts/issues-push-loop.sh' && echo "push-loop stopped" || echo "push-loop not running"

# Observability
# -----------------------------------------------------------------------------

[group('observe')]
[doc("Report loop state + queue depths")]
status:
  #!/usr/bin/env bash
  set -u
  if pgrep -f 'scripts/issues-eval-loop.sh' >/dev/null; then
    echo "eval-loop: running (pid $(pgrep -f 'scripts/issues-eval-loop.sh' | head -1))"
  else
    echo "eval-loop: not running"
  fi
  if pgrep -f 'scripts/issues-push-loop.sh' >/dev/null; then
    echo "push-loop: running (pid $(pgrep -f 'scripts/issues-push-loop.sh' | head -1))"
  else
    echo "push-loop: not running"
  fi
  echo "eval-inbox:   $(ls .issues/eval-inbox/ 2>/dev/null | wc -l | tr -d ' ') drafts"
  echo "pusher-inbox: $(ls .issues/pusher-inbox/ 2>/dev/null | wc -l | tr -d ' ') drafts"

[group('observe')]
[doc("Follow both logs, docker-compose style (prefix per source)")]
tail:
  #!/usr/bin/env bash
  set -u
  mkdir -p .sandbox
  touch .sandbox/eval.log .sandbox/push.log
  trap 'kill 0' EXIT INT TERM
  # Colors only when stdout is a TTY (piping to a file keeps plain text).
  if [ -t 1 ]; then
    C_EVAL=$'\033[36m'    # cyan
    C_PUSH=$'\033[35m'    # magenta
    C_DIM=$'\033[2m'
    C_OFF=$'\033[0m'
  else
    C_EVAL=''; C_PUSH=''; C_DIM=''; C_OFF=''
  fi
  color_lines() {
    local lc="$1" label="$2"
    awk -v lc="$lc" -v label="$label" -v dim="$C_DIM" -v off="$C_OFF" '{
      ts=""; rest=$0
      if (match($0, /^\[[0-9][0-9]:[0-9][0-9]:[0-9][0-9]\]/)) {
        ts=substr($0, RSTART, RLENGTH) " "
        rest=substr($0, RSTART+RLENGTH+1)
      }
      printf "%s%s%s %s|%s %s%s%s%s\n", lc, label, off, dim, off, dim, ts, off, rest
      fflush()
    }'
  }
  tail -n 10 -F .sandbox/eval.log 2>/dev/null | color_lines "$C_EVAL" "eval" &
  tail -n 10 -F .sandbox/push.log 2>/dev/null | color_lines "$C_PUSH" "push" &
  wait
