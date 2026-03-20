#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
# Dev wrapper — runs scampi from source via `just`.
# Symlink to ~/.local/bin/scampi for use outside the repo.
#
#   ln -s /path/to/scampi/scripts/scampi-dev.sh ~/.local/bin/scampi
#
set -eu

SCAMPI_ROOT="${SCAMPI_ROOT:-$(cd "$(dirname "$(readlink -f "$0" 2>/dev/null || realpath "$0")")/.." && pwd)}"

# Convert file arguments to absolute paths so just/go run
# resolve them correctly regardless of the caller's PWD.
args=""
for arg in "$@"; do
    if [ -f "${arg}" ]; then
        arg="$(cd "$(dirname "${arg}")" && pwd)/$(basename "${arg}")"
    fi
    args="${args} \"${arg}\""
done

eval exec just -f "\"${SCAMPI_ROOT}/justfile\"" scampi "${args}"
