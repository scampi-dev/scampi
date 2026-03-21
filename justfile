build_dir       := "./build"
bin_dir         := f"{{build_dir}}/bin"
bin_path        := f"{{bin_dir}}/scampi"
spdx_header     := "// SPDX-License-Identifier: GPL-3.0-only"
required_tools  := "shellcheck jq curl"
cross_targets   := "linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 freebsd/amd64 freebsd/arm64"


[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list {{args}}

# Daily drivers
# ##############################################################################

version  := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags  := "-s -w -X main.version=" + version

[doc("Build the scampi CLI binary")]
build:
  mkdir -p {{bin_dir}}
  go build -ldflags '{{ldflags}}' -o {{bin_path}} ./cmd/scampi

[doc("Cross-compile for all supported platforms (outdir=DIR prefix=NAME)")]
cross outdir=bin_dir prefix="scampi":
  #!/usr/bin/env bash
  set -euo pipefail
  mkdir -p "{{outdir}}"
  for pair in {{cross_targets}}; do
    os="${pair%/*}"
    arch="${pair#*/}"
    out="{{outdir}}/{{prefix}}-${os}-${arch}"
    printf "  %-20s → %s\n" "${os}/${arch}" "$out"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -ldflags '{{ldflags}}' -o "$out" ./cmd/scampi
  done

[doc("Build and run scampi locally")]
scampi *args:
  go run ./cmd/scampi {{args}}

[doc("Run tests (just test --list for subcommands)")]
mod test

[doc("Format all code")]
fmt:
  go fmt ./...
  ./scripts/fix-markdown-tables.py

[doc("Lint project")]
lint:
  go tool golangci-lint run
  go tool gomarklint
  go test -run TestMarkdownTableAlignment ./test/
  shellcheck scripts/*.sh
  just license-check

[doc("Site build/dev (just site --list for subcommands)")]
mod site

[doc("Codeberg repo management")]
mod cb 'codeberg.just'

[doc("Install external build/lint dependencies")]
setup:
  #!/usr/bin/env bash
  set -euo pipefail
  missing=()
  for cmd in {{required_tools}}; do
    command -v "$cmd" &>/dev/null || missing+=("$cmd")
  done
  if [[ ${#missing[@]} -eq 0 ]]; then
    echo "All dependencies installed."
    exit 0
  fi
  echo "Missing: ${missing[*]}"
  if command -v brew &>/dev/null; then
    brew install "${missing[@]}"
  elif command -v pacman &>/dev/null; then
    sudo pacman -S "${missing[@]}"
  elif command -v dnf &>/dev/null; then
    sudo dnf install -y "${missing[@]}"
  elif command -v apt-get &>/dev/null; then
    sudo apt-get install -y "${missing[@]}"
  else
    echo "Install manually: ${missing[*]}"
    exit 1
  fi

# Housekeeping
# ##############################################################################

[doc("Run code generation")]
generate:
  go generate ./...
  just _patch-license-headers

# Prepend SPDX header to generated files that stringer overwrites.
[private]
_patch-license-headers:
  #!/usr/bin/env bash
  set -euo pipefail
  header="{{spdx_header}}"
  while IFS= read -r f; do
    first=$(head -1 "$f")
    if [[ "$first" != "$header" ]]; then
      tmp=$(mktemp)
      { echo "$header"; echo; cat "$f"; } > "$tmp"
      mv "$tmp" "$f"
    fi
  done < <(find . -name '*_string.go' -not -path './vendor/*')

[doc("Check SPDX license headers")]
license-check:
  ./scripts/license-check.sh

[doc("Check for outdated direct dependencies")]
outdated:
  @./scripts/go-outdated.sh

[doc("Upgrade all direct dependencies to latest")]
upgrade:
  @./scripts/go-upgrade.sh

[doc("Clean project")]
clean:
  rm -rf {{build_dir}}
  go clean -testcache
