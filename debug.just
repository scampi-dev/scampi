# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list debug {{args}}

# Sandbox
# -----------------------------------------------------------------------------

[doc("Wipe sandbox state: truncate action log JSONL in place, drop /tmp/scampi-*")]
reset:
  @./scripts/sandbox-reset.sh

[doc("Reset sandbox state, then run scampi with given args (action log under .sandbox/log)")]
run *args:
  @./scripts/sandbox-reset.sh
  @go run ./cmd/scampi --action-log .sandbox/log {{args}}
