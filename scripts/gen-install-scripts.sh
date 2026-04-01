#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
# Generate install script variants for get.scampi.dev.
#
# Produces three files in site/static/ from scripts/install.sh:
#   install.sh  — both scampi + scampls  (served at /)
#   cli.sh      — scampi only            (served at /cli)
#   lsp.sh      — scampls only           (served at /lsp)
#
set -eu

SRC="scripts/install.sh"
OUT="site/static"

mkdir -p "${OUT}"

sed 's/##@@INSTALL_BINS@@##/install_one scampi\n  install_one scampls/' "${SRC}" >"${OUT}/install.sh"
sed 's/##@@INSTALL_BINS@@##/install_one scampi/' "${SRC}" >"${OUT}/cli.sh"
sed 's/##@@INSTALL_BINS@@##/install_one scampls/' "${SRC}" >"${OUT}/lsp.sh"
