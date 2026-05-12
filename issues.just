# scampi issue pipeline — two loops:
#   eval  (.issues/eval-inbox/  → claude-code QA → .issues/pusher-inbox/)
#   push  (.issues/pusher-inbox/ → Codeberg)

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

[doc("Stop both loops")]
down:
  @pkill -f 'scripts/issues-eval-loop.sh' && echo "eval-loop stopped" || echo "eval-loop not running"
  @pkill -f 'scripts/issues-push-loop.sh' && echo "push-loop stopped" || echo "push-loop not running"

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

[doc("Follow both logs, docker-compose style (prefix per source)")]
tail:
  #!/usr/bin/env bash
  set -u
  mkdir -p .sandbox
  touch .sandbox/eval.log .sandbox/push.log
  trap 'kill 0' EXIT INT TERM
  tail -n 10 -F .sandbox/eval.log 2>/dev/null | awk '{print "eval | " $0; fflush()}' &
  tail -n 10 -F .sandbox/push.log 2>/dev/null | awk '{print "push | " $0; fflush()}' &
  wait
