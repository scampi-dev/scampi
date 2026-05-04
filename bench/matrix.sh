#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# bench/matrix.sh — run bench/bench.sh across the (HOSTS, WAN_DELAY)
# scaling matrix. Each cell produces a separate result set under
# bench/results/, suffixed with .h<HOSTS>.{lan|wan<MS>} so the
# scenarios can be compared after the fact.
#
# Default matrix: HOSTS in (1, 3, 10), WAN_DELAY_MS in (0, 100).
# = 6 cells × (cold + warm) × (scampi + ansible) = 24 hyperfine runs.
#
# Provision once with the maximum host count before invoking:
#
#   BENCH_HOSTS=10 bash bench/provision.sh
#   bash bench/matrix.sh
#
# Tunables (env vars):
#   HOSTS_MATRIX="1 3 10"   — host counts to sweep
#   WAN_MATRIX="0 100"      — WAN_DELAY_MS values to sweep
#   RUNS=10                 — passed through to bench.sh

set -euo pipefail

# shellcheck source=bench/lib.sh
source "$(dirname "$0")/lib.sh"

require hyperfine
require_env_basics
require_scampi
require_ansible
export_ssh_env

HOSTS_MATRIX=${HOSTS_MATRIX:-"1 3 10"}
WAN_MATRIX=${WAN_MATRIX:-"0 100"}

# Validate that provision covered the largest host count we'll use.
max_hosts=0
for h in $HOSTS_MATRIX; do
    [[ "$h" -gt "$max_hosts" ]] && max_hosts=$h
done
top_vmid=$((BENCH_VMID_BASE + max_hosts - 1))
if ! ssh -o BatchMode=yes "${PVE_USER}@${PVE_HOST}" \
    "sudo pct status $top_vmid >/dev/null 2>&1"; then
    echo "matrix needs LXC vmid=$top_vmid (max HOSTS=$max_hosts) but it's not provisioned." >&2
    echo "run: BENCH_HOSTS=$max_hosts bash bench/provision.sh" >&2
    exit 1
fi

cell_count=0
for hosts in $HOSTS_MATRIX; do
    for wan in $WAN_MATRIX; do
        cell_count=$((cell_count + 1))
    done
done
echo "matrix: HOSTS in [$HOSTS_MATRIX] × WAN_DELAY_MS in [$WAN_MATRIX] = $cell_count cells"
echo

# Marker for "results written by this matrix run". Used to list cells
# at the end without depending on the results dir's mtime (which bumps
# with every new file).
marker=$(mktemp)
trap 'rm -f "$marker"' EXIT

cell=0
for hosts in $HOSTS_MATRIX; do
    for wan in $WAN_MATRIX; do
        cell=$((cell + 1))
        echo "==================== cell $cell/$cell_count: HOSTS=$hosts WAN=${wan}ms ===================="
        BENCH_HOSTS=$hosts WAN_DELAY_MS=$wan bash bench/bench.sh
        echo
    done
done

echo "matrix complete."
report=$(python3 bench/aggregate.py --since "$marker" --results-dir "$RESULTS_DIR" 2>&1)
echo "$report"
