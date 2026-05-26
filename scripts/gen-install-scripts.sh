#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
# Generate install script variants for get.scampi.dev.
#
# Produces three files in site/static/ from scripts/install.sh:
#   install.sh  — both scampi + scampls  (served at /)
#   cli.sh      — scampi only            (served at /cli)
#   lsp.sh      — scampls only           (served at /lsp)
#
# Also rewrites the block between the GENERATED:install.sh markers in
# site/content/get/install-source.md with a fenced rendering of the
# freshly generated install.sh. The prose and frontmatter around the
# markers are tracked in git and stay untouched.
#
set -eu

SRC="scripts/install.sh"
STATIC="site/static"
SOURCE_MD="site/content/get/install-source.md"
MARKER_OPEN='<!-- GENERATED:install.sh -->'
MARKER_CLOSE='<!-- /GENERATED:install.sh -->'

mkdir -p "${STATIC}"

sed 's/##@@INSTALL_BINS@@##/install_one scampi\n  install_one scampls/' "${SRC}" >"${STATIC}/install.sh"
sed 's/##@@INSTALL_BINS@@##/install_one scampi/' "${SRC}" >"${STATIC}/cli.sh"
sed 's/##@@INSTALL_BINS@@##/install_one scampls/' "${SRC}" >"${STATIC}/lsp.sh"

# Both markers must be present and intact before mutating.
if ! grep -qxF "${MARKER_OPEN}" "${SOURCE_MD}"; then
  echo "ERROR: ${SOURCE_MD} missing marker: ${MARKER_OPEN}" >&2
  exit 1
fi
if ! grep -qxF "${MARKER_CLOSE}" "${SOURCE_MD}"; then
  echo "ERROR: ${SOURCE_MD} missing marker: ${MARKER_CLOSE}" >&2
  exit 1
fi

# Rebuild everything between the markers as a bash-fenced block
# containing the rendered install.sh. The markers stay; content
# between them is replaced.
awk -v script="${STATIC}/install.sh" \
  -v mark_open="${MARKER_OPEN}" \
  -v mark_close="${MARKER_CLOSE}" '
  $0 == mark_open && !in_block {
    print
    print "```bash {filename=\"install.sh\" linenos=true}"
    while ((getline line < script) > 0) print line
    close(script)
    print "```"
    in_block = 1
    next
  }
  in_block && $0 == mark_close {
    print
    in_block = 0
    next
  }
  !in_block { print }
' "${SOURCE_MD}" >"${SOURCE_MD}.tmp"
mv "${SOURCE_MD}.tmp" "${SOURCE_MD}"
