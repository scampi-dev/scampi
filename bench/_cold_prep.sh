#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# bench/_cold_prep.sh — uncounted prep for each cold benchmark run.
#
# Invoked by hyperfine via --prepare; not for direct use. Reads env
# vars exported by bench/bench.sh (via lib.sh).
#
# Steps:
#   1. Roll every bench LXC back to the pristine snapshot
#   2. Start every LXC (rollback leaves them stopped)
#   3. Wait until the perf-user SSH listener is reachable on each
#   4. Apply / clear WAN netem on each LXC (after wait, so the wait
#      isn't slowed by the simulated delay)
#
# All four happen on every cold run; none of it is timed.

set -euo pipefail

# ----- 1+2: rollback + start in one SSH round-trip --------------------------

remote=""
for i in $(seq 0 $((BENCH_HOSTS - 1))); do
    vmid=$((BENCH_VMID_BASE + i))
    one="sudo pct rollback $vmid $SNAPSHOT_NAME && sudo pct start $vmid"
    if [[ -z "$remote" ]]; then remote="$one"; else remote="$remote && $one"; fi
done

ssh -o BatchMode=yes "${PVE_USER}@${PVE_HOST}" "$remote"

# ----- 3: wait for SSH on each LXC ------------------------------------------

for i in $(seq 0 $((BENCH_HOSTS - 1))); do
    ip="${BENCH_IP_PREFIX}.$((BENCH_IP_BASE + i))"
    until ssh \
        -i "$SCAMPI_BENCH_SSH_KEY" \
        -o BatchMode=yes \
        -o ConnectTimeout=2 \
        -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null \
        "${BENCH_USER}@${ip}" true 2>/dev/null
    do
        sleep 0.5
    done
done

# ----- 4: apply / clear WAN netem -------------------------------------------

bash bench/_apply_wan.sh
