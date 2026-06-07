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

[doc("Reset sandbox state, then run scampi with given args (state under .sandbox/state)")]
run *args:
  @./scripts/sandbox-reset.sh
  @SCAMPI_STATE_DIR=.sandbox/state go run ./cmd/scampi {{args}}
