#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
# Install scampi — https://scampi.dev
#
#   curl get.scampi.dev | sh
#
# Override install location:
#   curl get.scampi.dev | sh -s -- -d ~/.local/bin
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
API="https://api.github.com/repos/${REPO}"
DL="https://github.com/${REPO}/releases/download"

# Release signing key — embedded so verification works offline.
# To rotate: regenerate the keypair, push a new install.sh with the new
# pubkey, and re-sign existing releases.
SCAMPI_RELEASE_PUBKEY='releases@scampi.dev ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEDBbJOSWyfk9kJhHjUmSJVIax9lxGnOjwpL4dSheQfu'
SCAMPI_RELEASE_PRINCIPAL='releases@scampi.dev'

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

  asset="scampi-${OS}-${ARCH}"

  installed=""
  if command -v scampi >/dev/null 2>&1; then
    installed=$(scampi version 2>/dev/null | awk '{print $NF}')
  fi
  if [ "${installed}" = "${VERSION}" ]; then
    ok "scampi ${VERSION} already installed"
    INSTALL_DIR="$(dirname "$(command -v scampi)")"
  else
    [ -n "${installed}" ] && info "scampi: ${installed} → ${VERSION}"
    info "downloading ${asset}..."
    curl -fsSL -o "${TMPDIR}/scampi" "${DL}/${VERSION}/${asset}" ||
      fatal "download failed — scampi ${OS}/${ARCH} may not be available for ${VERSION}"
    verify_checksum "${asset}"
    do_install
  fi

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
    VERSION=$(curl -fsSL "${API}/releases?per_page=1" |
      grep -o '"tag_name":"[^"]*"' | head -1 | cut -d'"' -f4)
  fi

  if [ -z "${VERSION}" ]; then
    fatal "could not determine latest version"
  fi

  info "latest: ${VERSION}"
}

download_checksums() {
  SUMS_URL="${DL}/${VERSION}/SHA256SUMS"
  SIG_URL="${DL}/${VERSION}/SHA256SUMS.sig"
  TMPDIR=$(mktemp -d)
  trap 'rm -rf "${TMPDIR}"' EXIT

  curl -fsSL -o "${TMPDIR}/SHA256SUMS" "${SUMS_URL}" ||
    fatal "could not download checksums"

  # Pre-signing releases lack SHA256SUMS.sig; tolerate that for backward
  # compatibility. Any signature that IS present must verify.
  if curl -fsSL -o "${TMPDIR}/SHA256SUMS.sig" "${SIG_URL}" 2>/dev/null; then
    verify_signature
  else
    warn "release ${VERSION} is unsigned (no SHA256SUMS.sig)"
  fi
}

verify_signature() {
  if ! command -v ssh-keygen >/dev/null 2>&1; then
    fatal "release is signed but 'ssh-keygen' not found; install OpenSSH or download manually from ${DL}/${VERSION}/"
  fi

  printf '%s\n' "${SCAMPI_RELEASE_PUBKEY}" > "${TMPDIR}/allowed_signers"

  if ! ssh-keygen -Y verify \
      -f "${TMPDIR}/allowed_signers" \
      -I "${SCAMPI_RELEASE_PRINCIPAL}" \
      -n file \
      -s "${TMPDIR}/SHA256SUMS.sig" \
      < "${TMPDIR}/SHA256SUMS" >/dev/null 2>&1; then
    fatal "SHA256SUMS signature verification FAILED — refusing to install"
  fi
  ok "SHA256SUMS signature verified (${SCAMPI_RELEASE_PRINCIPAL})"
}

verify_checksum() {
  asset="$1"

  want=$(grep "${asset}" "${TMPDIR}/SHA256SUMS" | awk '{print $1}')
  if [ -z "${want}" ]; then
    fatal "no checksum found for ${asset}"
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "${TMPDIR}/scampi" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "${TMPDIR}/scampi" | awk '{print $1}')
  else
    warn "no sha256 tool found, skipping checksum verification"
    return
  fi

  if [ "${got}" != "${want}" ]; then
    fatal "checksum mismatch for ${asset}: expected ${want}, got ${got}"
  fi
  ok "scampi: checksum verified"
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
  chmod +x "${TMPDIR}/scampi"

  if [ -n "${DEST_DIR}" ]; then
    dest="${DEST_DIR}/scampi"
    confirm_overwrite "${dest}"
    mkdir -p "${DEST_DIR}"
    if [ -w "${DEST_DIR}" ]; then
      mv "${TMPDIR}/scampi" "${dest}"
    else
      info "installing scampi to ${dest} (requires sudo)"
      sudo mv "${TMPDIR}/scampi" "${dest}"
    fi
  elif [ -d "${HOME}/.local/bin" ]; then
    dest="${HOME}/.local/bin/scampi"
    confirm_overwrite "${dest}"
    mv "${TMPDIR}/scampi" "${dest}"
  elif [ -w "/usr/local/bin" ]; then
    dest="/usr/local/bin/scampi"
    confirm_overwrite "${dest}"
    mv "${TMPDIR}/scampi" "${dest}"
  else
    dest="/usr/local/bin/scampi"
    confirm_overwrite "${dest}"
    info "installing scampi to ${dest} (requires sudo)"
    sudo mv "${TMPDIR}/scampi" "${dest}"
  fi

  INSTALL_DIR="$(dirname "${dest}")"
  ok "scampi → ${dest}"
}

# PATH hint, after install.
path_hint() {
  case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) warn "add ${INSTALL_DIR} to your PATH" ;;
  esac
}

main "$@"
path_hint
