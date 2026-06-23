#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
# Fresh-clone bootstrap. Needs only a POSIX shell + curl: installs mise
# if it's missing, then installs the tool versions pinned in .mise.toml
# (go, just, shellcheck, jq). Run once after cloning:
#
#   ./scripts/bootstrap.sh
#
set -eu

cd "$(dirname "$0")/.."

if ! command -v mise >/dev/null 2>&1; then
  echo ":: mise not found — installing it (https://mise.jdx.dev)"
  curl -fsSL https://mise.run | sh
fi

# mise may not be on PATH yet in this shell; resolve the binary directly.
MISE="$(command -v mise 2>/dev/null || echo "${HOME}/.local/bin/mise")"
if [ ! -x "${MISE}" ]; then
  echo "could not locate mise after install — add ~/.local/bin to PATH and re-run" >&2
  exit 1
fi

echo ":: installing pinned tools from .mise.toml"
"${MISE}" install

cat <<'EOF'

Done — pinned go, just, shellcheck and jq are installed.

Activate mise once so they load automatically inside the repo:
  zsh:  echo 'eval "$(mise activate zsh)"'  >> ~/.zshrc  && exec zsh
  bash: echo 'eval "$(mise activate bash)"' >> ~/.bashrc && exec bash

Then build:  just build
EOF
