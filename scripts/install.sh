#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
# Install scampi — https://scampi.dev
#
#   curl -fsSL get.scampi.dev | sh
#   curl -fsSL get.scampi.dev | sh -s -- -o ~/.local/bin/myname
#
set -eu

BIN_NAME="scampi"
DEST_DIR=""

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
    -o | --output)
      [ $# -lt 2 ] && fatal "-o requires a path argument"
      DEST_DIR="$(dirname "$2")"
      BIN_NAME="$(basename "$2")"
      shift 2
      ;;
    *)
      fatal "unknown option: $1"
      ;;
    esac
  done
}
REPO="scampi-dev/scampi"
API="https://codeberg.org/api/v1/repos/${REPO}"
DL="https://codeberg.org/${REPO}/releases/download"

setup_colors() {
  if [ -t 1 ] && [ "${NO_COLOR:-}" = "" ]; then
    R='\033[0m' B='\033[1m' DIM='\033[2m'
    RED='\033[31m' GREEN='\033[32m' YELLOW='\033[33m' BLUE='\033[34m'
    ORANGE='\033[38;5;208m'
  else
    R='' B='' DIM='' RED='' GREEN='' YELLOW='' BLUE='' ORANGE=''
  fi
}

# shellcheck disable=SC2059
info() { printf "${BLUE}::${R} %s\n" "$1"; }
# shellcheck disable=SC2059
ok() { printf " ${GREEN}✓${R} %s\n" "$1"; }
# shellcheck disable=SC2059
warn() { printf " ${YELLOW}!${R} %s\n" "$1"; }

fatal() {
  # shellcheck disable=SC2059
  printf " ${RED}✗${R} %s\n" "$1" >&2
  exit 1
}

main() {
  parse_args "$@"
  setup_colors
  # shellcheck disable=SC2059
  printf "\n  ${ORANGE}${B}<('◡')⚙  get scampi${R} ${DIM}— https://scampi.dev${R}\n\n"
  detect_platform
  fetch_version
  check_installed
  download_and_verify
  install_binary
  # shellcheck disable=SC2059
  printf "\n  ${GREEN}${B}scampi ${VERSION}${R} installed to ${DIM}${DEST}${R}\n\n"
}

detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "${OS}" in
  linux) ;;
  darwin) ;;
  freebsd) ;;
  *) fatal "unsupported OS: ${OS}" ;;
  esac

  case "${ARCH}" in
  x86_64 | amd64) ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *) fatal "unsupported architecture: ${ARCH}" ;;
  esac

  info "platform: ${OS}/${ARCH}"
}

fetch_version() {
  # Codeberg's /releases/latest only returns stable releases.
  # Fall back to the first release (most recent) if no stable exists.
  VERSION=$(curl -fsSL "${API}/releases/latest" 2>/dev/null |
    grep -o '"tag_name":"[^"]*"' | head -1 | cut -d'"' -f4)

  if [ -z "${VERSION}" ]; then
    VERSION=$(curl -fsSL "${API}/releases?limit=1" |
      grep -o '"tag_name":"[^"]*"' | head -1 | cut -d'"' -f4)
  fi

  if [ -z "${VERSION}" ]; then
    fatal "could not determine latest version"
  fi

  info "latest: ${VERSION}"
}

check_installed() {
  installed=""

  # Try in PATH first
  if command -v "${BIN_NAME}" >/dev/null 2>&1; then
    installed=$("${BIN_NAME}" version 2>/dev/null | awk '{print $2}')
  fi

  # Check default install locations if not in PATH
  if [ -z "${installed}" ]; then
    for loc in "${HOME}/.local/bin/${BIN_NAME}" "/usr/local/bin/${BIN_NAME}"; do
      if [ -x "${loc}" ]; then
        installed=$("${loc}" version 2>/dev/null | awk '{print $2}')
        break
      fi
    done
  fi

  if [ "${installed}" = "${VERSION}" ]; then
    ok "scampi ${VERSION} is already installed"
    exit 0
  fi

  if [ -n "${installed}" ]; then
    info "found ${installed}, upgrading to ${VERSION}"
  fi
}

download_and_verify() {
  BINARY="scampi-${VERSION}-${OS}-${ARCH}"
  URL="${DL}/${VERSION}/${BINARY}"
  SUMS_URL="${DL}/${VERSION}/SHA256SUMS"

  TMPDIR=$(mktemp -d)
  trap 'rm -rf "${TMPDIR}"' EXIT

  info "downloading ${BINARY}..."
  curl -fsSL -o "${TMPDIR}/${BIN_NAME}" "${URL}" ||
    fatal "download failed — ${OS}/${ARCH} may not be available for ${VERSION}"
  curl -fsSL -o "${TMPDIR}/SHA256SUMS" "${SUMS_URL}" ||
    fatal "could not download checksums"

  verify_checksum
}

verify_checksum() {
  WANT=$(grep "${BINARY}" "${TMPDIR}/SHA256SUMS" | awk '{print $1}')
  if [ -z "${WANT}" ]; then
    fatal "no checksum found for ${BINARY}"
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    GOT=$(sha256sum "${TMPDIR}/${BIN_NAME}" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    GOT=$(shasum -a 256 "${TMPDIR}/${BIN_NAME}" | awk '{print $1}')
  else
    warn "no sha256 tool found, skipping checksum verification"
    return
  fi

  if [ "${GOT}" != "${WANT}" ]; then
    fatal "checksum mismatch: expected ${WANT}, got ${GOT}"
  fi
  ok "checksum verified"
}

confirm_overwrite() {
  if [ -e "$1" ] || [ -L "$1" ]; then
    if [ -L "$1" ]; then
      warn "$1 is a symlink → $(readlink "$1")"
    fi
    if [ -t 1 ] && [ -r /dev/tty ]; then
      printf "     overwrite %s? [y/N] " "$1"
      read -r answer </dev/tty
      case "${answer}" in
      [yY]) ;;
      *)
        echo "aborted."
        exit 0
        ;;
      esac
    fi
  fi
}

install_binary() {
  chmod +x "${TMPDIR}/${BIN_NAME}"

  if [ -n "${DEST_DIR}" ]; then
    # Explicit path via -o
    DEST="${DEST_DIR}/${BIN_NAME}"
    confirm_overwrite "${DEST}"
    if [ -w "${DEST_DIR}" ]; then
      mv "${TMPDIR}/${BIN_NAME}" "${DEST}"
    else
      info "installing to ${DEST} (requires sudo)"
      sudo mv "${TMPDIR}/${BIN_NAME}" "${DEST}"
    fi
  elif [ -d "${HOME}/.local/bin" ]; then
    DEST="${HOME}/.local/bin/${BIN_NAME}"
    confirm_overwrite "${DEST}"
    mv "${TMPDIR}/${BIN_NAME}" "${DEST}"
  elif [ -w "/usr/local/bin" ]; then
    DEST="/usr/local/bin/${BIN_NAME}"
    confirm_overwrite "${DEST}"
    mv "${TMPDIR}/${BIN_NAME}" "${DEST}"
  else
    DEST="/usr/local/bin/${BIN_NAME}"
    confirm_overwrite "${DEST}"
    info "installing to ${DEST} (requires sudo)"
    sudo mv "${TMPDIR}/${BIN_NAME}" "${DEST}"
  fi

  # PATH hint
  case ":${PATH}:" in
  *":$(dirname "${DEST}"):"*) ;;
  *) warn "add $(dirname "${DEST}") to your PATH" ;;
  esac
}

main "$@"
