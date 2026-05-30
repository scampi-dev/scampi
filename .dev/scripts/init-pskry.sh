#!/usr/bin/env bash
set -euo pipefail

SCRIPT_PATH="$(realpath "$0")"
SCRIPT_DIR="$(dirname "$SCRIPT_PATH")"
ROOT_DIR="$(realpath "$SCRIPT_DIR/../..")"

cd "$ROOT_DIR"

ln -sf .dev/nvim/lazy-pskry.lua .lazy.lua
ln -sf .dev/tmux/tmuxinator-pskry.yml .tmuxinator.yml
