#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
# Install scampi — https://scampi.dev
#
#   curl -fsSL get.scampi.dev | sh           # both scampi + scampls
#   curl -fsSL get.scampi.dev/cli | sh       # CLI only
#   curl -fsSL get.scampi.dev/lsp | sh       # LSP only
#
# Override install location:
#   curl -fsSL get.scampi.dev | sh -s -- -d ~/.local/bin
#
set -eu

DEST_DIR=""

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
    -d | --dir)
      [ $# -lt 2 ] && fatal "-d requires a directory argument"
      DEST_DIR="$2"
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
  download_checksums

  ##@@INSTALL_BINS@@##

  # shellcheck disable=SC2059
  printf "\n  ${GREEN}${B}${VERSION}${R} installed to ${DIM}${INSTALL_DIR}${R}\n\n"
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

download_checksums() {
  SUMS_URL="${DL}/${VERSION}/SHA256SUMS"
  TMPDIR=$(mktemp -d)
  trap 'rm -rf "${TMPDIR}"' EXIT

  curl -fsSL -o "${TMPDIR}/SHA256SUMS" "${SUMS_URL}" ||
    fatal "could not download checksums"
}

install_one() {
  bin_name="$1"
  asset="${bin_name}-${OS}-${ARCH}"
  url="${DL}/${VERSION}/${asset}"

  # Check if already installed at this version.
  installed=""
  if command -v "${bin_name}" >/dev/null 2>&1; then
    installed=$("${bin_name}" version 2>/dev/null | awk '{print $NF}')
  fi
  if [ "${installed}" = "${VERSION}" ]; then
    ok "${bin_name} ${VERSION} already installed"
    return
  fi
  if [ -n "${installed}" ]; then
    info "${bin_name}: ${installed} → ${VERSION}"
  fi

  info "downloading ${asset}..."
  curl -fsSL -o "${TMPDIR}/${bin_name}" "${url}" ||
    fatal "download failed — ${bin_name} ${OS}/${ARCH} may not be available for ${VERSION}"

  verify_checksum "${bin_name}" "${asset}"
  do_install "${bin_name}"
}

verify_checksum() {
  bin_name="$1"
  asset="$2"

  want=$(grep "${asset}" "${TMPDIR}/SHA256SUMS" | awk '{print $1}')
  if [ -z "${want}" ]; then
    fatal "no checksum found for ${asset}"
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "${TMPDIR}/${bin_name}" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "${TMPDIR}/${bin_name}" | awk '{print $1}')
  else
    warn "no sha256 tool found, skipping checksum verification"
    return
  fi

  if [ "${got}" != "${want}" ]; then
    fatal "checksum mismatch for ${asset}: expected ${want}, got ${got}"
  fi
  ok "${bin_name}: checksum verified"
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

do_install() {
  bin_name="$1"
  chmod +x "${TMPDIR}/${bin_name}"

  if [ -n "${DEST_DIR}" ]; then
    dest="${DEST_DIR}/${bin_name}"
    confirm_overwrite "${dest}"
    mkdir -p "${DEST_DIR}"
    if [ -w "${DEST_DIR}" ]; then
      mv "${TMPDIR}/${bin_name}" "${dest}"
    else
      info "installing ${bin_name} to ${dest} (requires sudo)"
      sudo mv "${TMPDIR}/${bin_name}" "${dest}"
    fi
  elif [ -d "${HOME}/.local/bin" ]; then
    dest="${HOME}/.local/bin/${bin_name}"
    confirm_overwrite "${dest}"
    mv "${TMPDIR}/${bin_name}" "${dest}"
  elif [ -w "/usr/local/bin" ]; then
    dest="/usr/local/bin/${bin_name}"
    confirm_overwrite "${dest}"
    mv "${TMPDIR}/${bin_name}" "${dest}"
  else
    dest="/usr/local/bin/${bin_name}"
    confirm_overwrite "${dest}"
    info "installing ${bin_name} to ${dest} (requires sudo)"
    sudo mv "${TMPDIR}/${bin_name}" "${dest}"
  fi

  INSTALL_DIR="$(dirname "${dest}")"
  ok "${bin_name} → ${dest}"
}

# PATH hint (once, after all installs)
path_hint() {
  case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) warn "add ${INSTALL_DIR} to your PATH" ;;
  esac
}

main "$@"
path_hint
