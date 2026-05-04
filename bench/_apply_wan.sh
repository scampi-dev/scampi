#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# bench/_apply_wan.sh — apply or clear netem on each bench LXC's
# eth0 to simulate WAN egress delay.
#
# Behaviour:
#   WAN_DELAY_MS == 0  → ensure no qdisc on eth0 (idempotent)
#   WAN_DELAY_MS  > 0  → install netem with delay <WAN_DELAY_MS>ms
#
# Always called by _cold_prep.sh after the wait-for-ssh step (so the
# wait itself isn't slowed by the simulated delay) and once at the
# top of bench.sh (so warm-only runs still apply the requested
# WAN setting).
#
# pct exec runs as root inside the LXC; tc lives at /sbin/tc and is
# part of iproute2 (preinstalled on Debian standard).

set -euo pipefail

if [[ "${WAN_DELAY_MS:-0}" -gt 0 ]]; then
    # Delete any existing qdisc, then add netem with the requested
    # delay. The first command is allowed to fail (no qdisc yet).
    inner="tc qdisc del dev eth0 root 2>/dev/null; tc qdisc add dev eth0 root netem delay ${WAN_DELAY_MS}ms"
else
    # Strip any leftover qdisc from a previous WAN scenario.
    inner="tc qdisc del dev eth0 root 2>/dev/null || true"
fi

cmd=""
for i in $(seq 0 $((BENCH_HOSTS - 1))); do
    vmid=$((BENCH_VMID_BASE + i))
    one="sudo pct exec $vmid -- bash -c '$inner'"
    if [[ -z "$cmd" ]]; then cmd="$one"; else cmd="$cmd && $one"; fi
done

ssh -o BatchMode=yes "${PVE_USER}@${PVE_HOST}" "$cmd"
