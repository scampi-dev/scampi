#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# bench/lib.sh — shared helpers for bench/{provision,bench}.sh.
# Sourced, never executed directly. Loads bench/.env, defines the
# require_* helpers, sets up SSH key paths.
#
# Note: the cd to repo root happens here, so all paths in callers
# (bench/provision/*.scampi, bench/scampi/deploy.scampi, etc.) are
# repo-root-relative regardless of where the caller was invoked from.

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    echo "lib.sh is sourced, not executed — see bench/{provision,bench}.sh" >&2
    exit 1
fi

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

# ----- load environment -----------------------------------------------------

if [[ ! -f bench/.env.example ]]; then
    echo "missing bench/.env.example — should be committed to the repo" >&2
    exit 1
fi

set -a
# shellcheck disable=SC1091
source bench/.env.example
if [[ -f bench/.env ]]; then
    # shellcheck disable=SC1091
    source bench/.env
fi
set +a

# ----- knobs not in .env (paths to tools and local artifacts) ---------------

RESULTS_DIR=${RESULTS_DIR:-bench/results}
SCAMPI=${SCAMPI:-./build/bin/scampi}
ANSIBLE_VENV=${ANSIBLE_VENV:-bench/ansible/venv}
ANSIBLE="${ANSIBLE_VENV}/bin/ansible-playbook"

SSH_KEY_DIR=${SSH_KEY_DIR:-bench/.ssh}
SSH_KEY_FILE=${SSH_KEY_FILE:-$SSH_KEY_DIR/scampi_bench}

# ----- helpers --------------------------------------------------------------

require() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "missing required tool: $1" >&2
        exit 1
    fi
}

require_var() {
    local name="$1"
    if [[ -z "${!name:-}" ]]; then
        echo "required env var $name is unset — set it in bench/.env" >&2
        exit 1
    fi
}

# Validate the env vars used by every script in this dir.
require_env_basics() {
    require ssh
    require_var PVE_HOST
    require_var PVE_NODE
    require_var PVE_GATEWAY
    require_var BENCH_IP_PREFIX
    require_var BENCH_VMID_BASE
    require_var BENCH_IP_BASE
    require_var BENCH_USER
    require_var SNAPSHOT_NAME
    require_var BENCH_HOSTS
}

require_scampi() {
    if [[ ! -x "$SCAMPI" ]]; then
        echo "scampi binary not found at $SCAMPI — run 'just build' first" >&2
        exit 1
    fi
}

require_ansible() {
    if [[ ! -x "$ANSIBLE" ]]; then
        echo "ansible venv not found at $ANSIBLE — run bash bench/provision.sh first" >&2
        exit 1
    fi
}

# Make the bench SSH keypair + private/public env vars visible to
# child processes (provision .scampi files via std.env, ansible's
# inventory, etc.).
#
# SCAMPI_BENCH_SSH_KEY must be an absolute path — scampi resolves
# the ssh.target `key` field relative to the .scampi file's
# directory, not the current working directory.
export_ssh_env() {
    if [[ ! -f "${SSH_KEY_FILE}.pub" ]]; then
        echo "missing bench ssh key at ${SSH_KEY_FILE}.pub — run bash bench/provision.sh first" >&2
        exit 1
    fi
    SCAMPI_BENCH_SSH_PUB="$(cat "${SSH_KEY_FILE}.pub")"
    case "$SSH_KEY_FILE" in
        /*) SCAMPI_BENCH_SSH_KEY="$SSH_KEY_FILE" ;;
        *)  SCAMPI_BENCH_SSH_KEY="$(pwd)/$SSH_KEY_FILE" ;;
    esac
    export SCAMPI_BENCH_SSH_PUB SCAMPI_BENCH_SSH_KEY
}
