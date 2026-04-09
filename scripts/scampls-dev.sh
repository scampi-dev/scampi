#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Dev wrapper — rebuilds if source changed, then runs scampls.
# Point your Neovim LSP config cmd at this script.
set -euo pipefail
SRCDIR="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"
exec "$SRCDIR/scripts/dev-run.sh" scampls "$@"
